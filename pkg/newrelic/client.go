package newrelic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const (
	BaseHost        = "api.newrelic.com"
	GraphQHEndpoint = "/graphql"
)

type Client struct {
	AccountId  int
	httpClient *http.Client
	apikey     string
	baseURL    *url.URL
}

func NewClient(ctx context.Context, httpClient *http.Client, apikey string) (*Client, error) {
	u := &url.URL{
		Scheme: "https",
		Host:   BaseHost,
		Path:   GraphQHEndpoint,
	}

	accId, err := GetAccountId(ctx, httpClient, u.String(), apikey)
	if err != nil {
		return nil, err
	}

	return &Client{
		httpClient: httpClient,
		apikey:     apikey,
		baseURL:    u,
		AccountId:  accId,
	}, nil
}

func GetAccountId(ctx context.Context, httpClient *http.Client, url string, apikey string) (int, error) {
	var res AccountsResponse
	q := composeAccountsQuery()

	body := &GraphqlBody{
		Query: q,
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("API-Key", apikey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, fmt.Errorf("failed to decode response body: %w", err)
	}

	accounts := res.Data.Actor.Accounts
	if len(accounts) == 0 {
		return 0, fmt.Errorf("no accounts found")
	}

	// TODO: support multiple accounts (only available in enterprise plan)
	return accounts[0].ID, nil
}

// ListUsers return users across whole organization.
func (c *Client) ListUsers(ctx context.Context, cursor string) ([]User, string, error) {
	var res UsersResponse
	variables := map[string]interface{}{}

	if cursor != "" {
		variables["userCursor"] = cursor
	}

	err := c.doRequest(
		ctx,
		composeUsersQuery(),
		variables,
		&res,
	)
	if err != nil {
		return nil, "", err
	}

	return res.Data.Actor.Users.Search.Users,
		res.Data.Actor.Users.Search.NextCursor,
		nil
}

// GetOrg returns organization details.
func (c *Client) GetOrg(ctx context.Context) (*Org, error) {
	var res OrgDetailResponse

	err := c.doRequest(ctx, composeOrgQuery(), nil, &res)
	if err != nil {
		return nil, err
	}

	return &res.Data.Actor.Organization, nil
}

// ListRoles returns roles across whole organization.
func (c *Client) ListRoles(ctx context.Context, cursor string) ([]Role, string, error) {
	var res RolesResponse
	variables := map[string]interface{}{}

	if cursor != "" {
		variables["roleCursor"] = cursor
	}

	err := c.doRequest(
		ctx,
		composeRolesQuery(),
		variables,
		&res,
	)
	if err != nil {
		return nil, "", err
	}

	return res.Data.Actor.Organization.Management.Roles.Roles,
		res.Data.Actor.Organization.Management.Roles.NextCursor,
		nil
}

// ListGroupsWithRole returns groups with specified role under specified domain.
func (c *Client) ListGroupsWithRole(ctx context.Context, domainId, roleId, cursor string) ([]Group, string, error) {
	var res GroupsResponse
	variables := map[string]interface{}{
		"domainId": domainId,
		"roleId":   roleId,
	}

	// set variables for pagination
	if cursor != "" {
		variables["groupCursor"] = cursor
	}

	err := c.doRequest(
		ctx,
		composeAllGroupsWithRoleQuery(),
		variables,
		&res,
	)
	if err != nil {
		return nil, "", err
	}

	domains := res.Data.Actor.Organization.Management.Domains

	if len(domains.Domains) == 0 {
		return nil, "", fmt.Errorf("domain not found: %s", domainId)
	}

	if len(domains.Domains) > 1 {
		return nil, "", fmt.Errorf("invalid id(%s) or cursor(%s), found more domains", domainId, cursor)
	}

	groups := domains.Domains[0].Groups.Groups

	return groups, domains.NextCursor, nil
}

// ListDomains returns all authentication domains across organization.
func (c *Client) ListDomains(ctx context.Context, cursor string) ([]Domain, string, error) {
	var res OrgAuthManagementResponse[struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Groups struct {
			Total int `json:"totalCount"`
		}
	}]
	variables := map[string]interface{}{}

	// set variables for pagination
	if cursor != "" {
		variables["cursor"] = cursor
	}

	err := c.doRequest(
		ctx,
		composeDomainsQuery(),
		variables,
		&res,
	)
	if err != nil {
		return nil, "", err
	}

	var ad []Domain
	nextDomains := res.Data.Actor.Organization.Management.Domains.NextCursor
	domains := res.Data.Actor.Organization.Management.Domains.Domains
	for _, d := range domains {
		domain := Domain{
			ID:    d.ID,
			Name:  d.Name,
			Total: d.Groups.Total,
		}

		ad = append(
			ad,
			domain,
		)
	}

	return ad, nextDomains, nil
}

// ListGroups returns groups with roles under specific domain.
func (c *Client) ListGroups(ctx context.Context, domainId, cursor string) ([]Group, string, error) {
	var res GroupsResponse
	variables := map[string]interface{}{
		"domainId": domainId,
	}

	// set variables for pagination
	if cursor != "" {
		variables["groupCursor"] = cursor
	}

	err := c.doRequest(
		ctx,
		composeGroupsQuery(),
		variables,
		&res,
	)
	if err != nil {
		return nil, "", err
	}

	domains := res.Data.Actor.Organization.Management.Domains
	if len(domains.Domains) == 0 {
		return nil, "", fmt.Errorf("domain not found: %s", domainId)
	}

	if len(domains.Domains) > 1 {
		return nil, "", fmt.Errorf("invalid id(%s) or cursor(%s), found more domains", domainId, cursor)
	}

	groups := domains.Domains[0].Groups.Groups

	return groups, domains.NextCursor, nil
}

// ListGroupMembers returns users under specific group.
func (c *Client) ListGroupMembers(ctx context.Context, domainId, groupId, cursor string) ([]string, string, error) {
	var res GroupMembersResponse
	variables := map[string]interface{}{
		"domainId": domainId,
		"groupId":  groupId,
	}

	if cursor != "" {
		variables["membersCursor"] = cursor
	}

	err := c.doRequest(
		ctx,
		composeGroupMembersQuery(),
		variables,
		&res,
	)
	if err != nil {
		return nil, "", err
	}

	domains := res.Data.Actor.Organization.Management.Domains
	if len(domains.Domains) == 0 {
		return nil, "", fmt.Errorf("domain not found: %s", domainId)
	}

	if len(domains.Domains) > 1 {
		return nil, "", fmt.Errorf("invalid id(%s) or cursor(%s), found more domains", domainId, cursor)
	}

	if len(domains.Domains[0].Groups.Groups) == 0 {
		return nil, "", fmt.Errorf("group not found: %s", groupId)
	}

	if len(domains.Domains[0].Groups.Groups) > 1 {
		return nil, "", fmt.Errorf("invalid id(%s) or cursor(%s), found more groups", groupId, cursor)
	}

	var users []string

	// loop through domains if there is group with the same id
	for _, d := range domains.Domains {
		for _, g := range d.Groups.Groups {
			for _, u := range g.Users.Users {
				users = append(users, u.ID)
			}
		}
	}

	return users, domains.Domains[0].Groups.Groups[0].Users.NextCursor, nil
}

func (c *Client) AddUserToGroup(ctx context.Context, groupId, userId string) error {
	var res AddGroupMemberResponse
	variables := map[string]interface{}{
		"groupId": groupId,
		"userId":  userId,
	}

	err := c.doRequest(
		ctx,
		composeAddGroupMemberMutation(),
		variables,
		&res,
	)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) RemoveUserFromGroup(ctx context.Context, groupId, userId string) error {
	var res RemoveGroupMemberResponse
	variables := map[string]interface{}{
		"groupId": groupId,
		"userId":  userId,
	}

	err := c.doRequest(
		ctx,
		composeRemoveGroupMemberMutation(),
		variables,
		&res,
	)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) AddGroupRole(ctx context.Context, roleId, groupId string) error {
	var res GrantRoleResponse
	variables := map[string]interface{}{
		"groupId": groupId,
		"roleId":  roleId,
	}

	err := c.doRequest(
		ctx,
		composeAddGroupRoleMutation(),
		variables,
		&res,
	)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) AddAccountRole(ctx context.Context, roleId, groupId string, accountId int) error {
	var res GrantRoleResponse
	variables := map[string]interface{}{
		"accountId": accountId,
		"groupId":   groupId,
		"roleId":    roleId,
	}

	err := c.doRequest(
		ctx,
		composeAddAccountRoleMutation(),
		variables,
		&res,
	)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) AddOrgRole(ctx context.Context, roleId, groupId string) error {
	var res GrantRoleResponse
	variables := map[string]interface{}{
		"roleId":  roleId,
		"groupId": groupId,
	}

	err := c.doRequest(
		ctx,
		composeAddOrgRoleMutation(),
		variables,
		&res,
	)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) RemoveGroupRole(ctx context.Context, roleId, groupId string) error {
	var res RevokeRoleResponse
	variables := map[string]interface{}{
		"groupId": groupId,
		"roleId":  roleId,
	}

	err := c.doRequest(
		ctx,
		composeRemoveGroupRoleMutation(),
		variables,
		&res,
	)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) RemoveAccountRole(ctx context.Context, roleId, groupId string, accountId int) error {
	var res RevokeRoleResponse
	variables := map[string]interface{}{
		"accountId": accountId,
		"roleId":    roleId,
		"groupId":   groupId,
	}

	err := c.doRequest(
		ctx,
		composeRemoveAccountRoleMutation(),
		variables,
		&res,
	)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) RemoveOrgRole(ctx context.Context, roleId, groupId string) error {
	var res RevokeRoleResponse
	variables := map[string]interface{}{
		"roleId":  roleId,
		"groupId": groupId,
	}

	err := c.doRequest(
		ctx,
		composeRemoveOrgRoleMutation(),
		variables,
		&res,
	)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) doRequest(ctx context.Context, q string, v map[string]interface{}, res interface{}) error {
	body := &GraphqlBody{
		Query:     q,
		Variables: v,
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL.String(),
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("API-Key", c.apikey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(res); err != nil {
		return fmt.Errorf("failed to decode response body: %w", err)
	}

	return nil
}
