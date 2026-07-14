package newrelic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	BaseHost        = "api.newrelic.com"
	GraphQHEndpoint = "/graphql"

	domainIDKey = "domainId"
	roleIDKey   = "roleId"
	groupIDKey  = "groupId"
	userIDKey   = "userId"
	emailKey    = "email"
	nameKey     = "name"
)

type Client struct {
	AccountId  int
	httpClient *http.Client
	apikey     string
	baseURL    *url.URL
}

func NewClient(ctx context.Context, httpClient *http.Client, apikey string, baseURL string) (*Client, error) {
	var u *url.URL
	if baseURL != "" {
		var err error
		u, err = url.Parse(baseURL)
		if err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
	} else {
		u = &url.URL{
			Scheme: "https",
			Host:   BaseHost,
			Path:   GraphQHEndpoint,
		}
	}

	var accId int
	var err error
	if httpClient != nil {
		accId, err = GetAccountId(ctx, httpClient, u.String(), apikey)
		if err != nil {
			return nil, err
		}
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

// ListUsers returns users across the organization via the NerdGraph v2
// user-management query. Resource IDs are always v2 identity ids (required by
// userManagementDeleteUser and other mutations), including when zero or multiple
// authentication domains exist.
func (c *Client) ListUsers(ctx context.Context, domainId string, cursor string) ([]User, string, error) {
	var resV2 UsersResponseV2
	variables := map[string]interface{}{}
	if cursor != "" {
		variables["userCursor"] = cursor
	}
	if domainId != "" {
		variables["domainId"] = domainId
	}

	if err := c.getResponse(ctx, composeUsersQueryV2, variables, &resV2); err != nil {
		return nil, "", err
	}

	var (
		users      []User
		nextCursor string
	)
	for _, domain := range resV2.Data.Actor.Organization.UserManagement.AuthenticationDomains.AuthenticationDomains {
		for _, user := range domain.Users.Users {
			users = append(users, User{
				Name:  user.Name,
				Email: user.Email,
				ID:    user.ID,
			})
		}
		if nextCursor == "" && domain.Users.NextCursor != "" {
			nextCursor = domain.Users.NextCursor
		}
	}

	return users, nextCursor, nil
}

func (c *Client) getResponse(ctx context.Context, query func() string, variables map[string]interface{}, res interface{}) error {
	return c.doReadRequest(ctx, query(), variables, &res)
}

// GetOrg returns organization details.
func (c *Client) GetOrg(ctx context.Context) (*Org, error) {
	var res OrgDetailResponse

	if err := c.doReadRequest(ctx, composeOrgQuery(), nil, &res); err != nil {
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

	if err := c.doReadRequest(ctx, composeRolesQuery(), variables, &res); err != nil {
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
		domainIDKey: domainId,
		roleIDKey:   roleId,
	}

	// set variables for pagination
	if cursor != "" {
		variables["groupCursor"] = cursor
	}

	if err := c.doReadRequest(ctx, composeAllGroupsWithRoleQuery(), variables, &res); err != nil {
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

	if err := c.doReadRequest(ctx, composeDomainsQuery(), variables, &res); err != nil {
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
		domainIDKey: domainId,
	}

	// set variables for pagination
	if cursor != "" {
		variables["groupCursor"] = cursor
	}

	if err := c.doReadRequest(ctx, composeGroupsQuery(), variables, &res); err != nil {
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
		domainIDKey: domainId,
		groupIDKey:  groupId,
	}

	if cursor != "" {
		variables["membersCursor"] = cursor
	}

	if err := c.doReadRequest(ctx, composeGroupMembersQuery(), variables, &res); err != nil {
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
		groupIDKey: groupId,
		userIDKey:  userId,
	}

	body := &GraphqlBody{
		Query:     composeAddGroupMemberMutation(),
		Variables: variables,
	}
	if err := doRawRequest(ctx, c.httpClient, c.baseURL.String(), c.apikey, body, &res); err != nil {
		return err
	}
	if len(res.Errors) > 0 {
		if isAlreadyMemberErr(res.Errors[0]) {
			return nil
		}
		return fmt.Errorf("baton-newrelic: add user to group failed: %s", res.Errors[0].Message)
	}
	return nil
}

func (c *Client) RemoveUserFromGroup(ctx context.Context, groupId, userId string) error {
	var res RemoveGroupMemberResponse
	variables := map[string]interface{}{
		groupIDKey: groupId,
		userIDKey:  userId,
	}

	body := &GraphqlBody{
		Query:     composeRemoveGroupMemberMutation(),
		Variables: variables,
	}
	if err := doRawRequest(ctx, c.httpClient, c.baseURL.String(), c.apikey, body, &res); err != nil {
		return err
	}
	if len(res.Errors) > 0 {
		if isNotFoundErr(res.Errors[0]) {
			return nil
		}
		return fmt.Errorf("baton-newrelic: remove user from group failed: %s", res.Errors[0].Message)
	}
	return nil
}

func (c *Client) AddGroupRole(ctx context.Context, roleId, groupId string) error {
	var res GrantRoleResponse
	variables := map[string]interface{}{
		groupIDKey: groupId,
		roleIDKey:  roleId,
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
		roleIDKey:  roleId,
		groupIDKey: groupId,
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
		groupIDKey: groupId,
		roleIDKey:  roleId,
	}

	body := &GraphqlBody{
		Query:     composeRemoveGroupRoleMutation(),
		Variables: variables,
	}
	if err := doRawRequest(ctx, c.httpClient, c.baseURL.String(), c.apikey, body, &res); err != nil {
		return err
	}
	if len(res.Errors) > 0 {
		if isNotFoundErr(res.Errors[0]) {
			return nil
		}
		return fmt.Errorf("baton-newrelic: remove group role failed: %s", res.Errors[0].Message)
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

	body := &GraphqlBody{
		Query:     composeRemoveAccountRoleMutation(),
		Variables: variables,
	}
	if err := doRawRequest(ctx, c.httpClient, c.baseURL.String(), c.apikey, body, &res); err != nil {
		return err
	}
	if len(res.Errors) > 0 {
		if isNotFoundErr(res.Errors[0]) {
			return nil
		}
		return fmt.Errorf("baton-newrelic: remove account role failed: %s", res.Errors[0].Message)
	}
	return nil
}

func (c *Client) RemoveOrgRole(ctx context.Context, roleId, groupId string) error {
	var res RevokeRoleResponse
	variables := map[string]interface{}{
		roleIDKey:  roleId,
		groupIDKey: groupId,
	}

	body := &GraphqlBody{
		Query:     composeRemoveOrgRoleMutation(),
		Variables: variables,
	}
	if err := doRawRequest(ctx, c.httpClient, c.baseURL.String(), c.apikey, body, &res); err != nil {
		return err
	}
	if len(res.Errors) > 0 {
		if isNotFoundErr(res.Errors[0]) {
			return nil
		}
		return fmt.Errorf("baton-newrelic: remove org role failed: %s", res.Errors[0].Message)
	}
	return nil
}

// GetUserByEmail returns the user with the given email using a single filtered
// NerdGraph v2 query. The returned User.ID is the v2 identity id required by
// user-management mutations. Returns nil, nil if no match is found.
func (c *Client) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var res GetUserByEmailResponse
	body := &GraphqlBody{
		Query:     composeGetUserByEmailQuery(),
		Variables: map[string]interface{}{emailKey: email},
	}
	if err := doRawRequest(ctx, c.httpClient, c.baseURL.String(), c.apikey, body, &res); err != nil {
		return nil, fmt.Errorf("baton-newrelic: GetUserByEmail request failed: %w", err)
	}
	if len(res.Errors) > 0 {
		return nil, fmt.Errorf("baton-newrelic: GetUserByEmail failed: %s", res.Errors[0].Message)
	}
	for _, domain := range res.Data.Actor.Organization.UserManagement.AuthenticationDomains.AuthenticationDomains {
		for _, u := range domain.Users.Users {
			if strings.EqualFold(u.Email, email) {
				return &User{ID: u.ID, Email: u.Email, Name: u.Name}, nil
			}
		}
	}
	return nil, nil
}

// CreateUser creates a new user in the given authentication domain.
func (c *Client) CreateUser(ctx context.Context, authDomainId, email, name, userType string) (string, error) {
	var res CreateUserResponse
	variables := map[string]interface{}{
		"authDomainId": authDomainId,
		emailKey:       email,
		nameKey:        name,
		"userType":     userType,
	}

	body := &GraphqlBody{
		Query:     composeCreateUserMutation(),
		Variables: variables,
	}
	if err := doRawRequest(ctx, c.httpClient, c.baseURL.String(), c.apikey, body, &res); err != nil {
		return "", err
	}

	if len(res.Errors) > 0 {
		return "", fmt.Errorf("baton-newrelic: create user failed: %s", res.Errors[0].Message)
	}

	return res.Data.UserManagementCreateUser.CreatedUser.ID, nil
}

// UpdateUser updates an existing user's attributes (any of email, name, userType may be empty to skip).
// Only non-empty fields are included in the mutation so absent fields are left unchanged.
func (c *Client) UpdateUser(ctx context.Context, userId, email, name, userType string) error {
	variables := map[string]interface{}{
		userIDKey: userId,
	}
	var updateFields []string
	if email != "" {
		variables[emailKey] = email
		updateFields = append(updateFields, emailKey)
	}
	if name != "" {
		variables[nameKey] = name
		updateFields = append(updateFields, nameKey)
	}
	if userType != "" {
		variables["userType"] = userType
		updateFields = append(updateFields, "userType")
	}

	var res UpdateUserResponse
	body := &GraphqlBody{
		Query:     composeUpdateUserMutation(updateFields),
		Variables: variables,
	}
	if err := doRawRequest(ctx, c.httpClient, c.baseURL.String(), c.apikey, body, &res); err != nil {
		return err
	}
	if len(res.Errors) > 0 {
		return fmt.Errorf("baton-newrelic: update user failed: %s", res.Errors[0].Message)
	}
	return nil
}

// DeleteUser permanently deletes a user. Returns nil if user is not found (idempotent).
func (c *Client) DeleteUser(ctx context.Context, userId string) error {
	variables := map[string]interface{}{
		userIDKey: userId,
	}

	var res DeleteUserResponse
	body := &GraphqlBody{
		Query:     composeDeleteUserMutation(),
		Variables: variables,
	}
	if err := doRawRequest(ctx, c.httpClient, c.baseURL.String(), c.apikey, body, &res); err != nil {
		return err
	}
	if len(res.Errors) > 0 {
		if isNotFoundErr(res.Errors[0]) {
			return nil
		}
		return fmt.Errorf("baton-newrelic: delete user failed: %s", res.Errors[0].Message)
	}
	return nil
}

// isNotFoundErr reports whether a NerdGraph error represents a "not found" condition.
// It checks the errorClass extension first (preferred) and falls back to message
// substring matching, including NR's literal not-found message.
func isNotFoundErr(e GraphqlError) bool {
	if class, ok := e.Extensions["errorClass"].(string); ok && class == "NOT_FOUND" {
		return true
	}
	lower := strings.ToLower(e.Message)
	return strings.Contains(lower, "not found") ||
		strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "no user") ||
		strings.Contains(lower, "could not find the target")
}

// isAlreadyMemberErr reports whether a NerdGraph error represents an "already a member"
// condition for group membership mutations — treated as success for idempotency.
func isAlreadyMemberErr(e GraphqlError) bool {
	if class, ok := e.Extensions["errorClass"].(string); ok {
		switch class {
		case "DUPLICATE", "ALREADY_EXISTS", "CONFLICT":
			return true
		}
	}
	lower := strings.ToLower(e.Message)
	return strings.Contains(lower, "already a member") ||
		strings.Contains(lower, "already in the group") ||
		strings.Contains(lower, "already added") ||
		strings.Contains(lower, "duplicate")
}

func doRawRequest(ctx context.Context, httpClient *http.Client, rawURL, apikey string, body *GraphqlBody, res interface{}) error {
	reqBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("baton-newrelic: failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("baton-newrelic: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("API-Key", apikey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("baton-newrelic: unexpected status code: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(res); err != nil {
		return fmt.Errorf("baton-newrelic: failed to decode response: %w", err)
	}
	return nil
}

// doReadRequest executes a GraphQL query and tolerates top-level errors alongside
// partial data. NerdGraph can return usable list data together with non-fatal
// field-level errors on read/list paths; aborting on any error would discard
// valid results. For mutations, use doRequest which fails strictly on errors.
//
// Total failures — NerdGraph HTTP 200 with data=null and a non-empty errors array
// (e.g. auth/permission denied) — are returned as errors so callers never
// silently treat a failed read as an empty page.
func (c *Client) doReadRequest(ctx context.Context, q string, v map[string]interface{}, res interface{}) error {
	body := &GraphqlBody{
		Query:     q,
		Variables: v,
	}
	reqBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("baton-newrelic: failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL.String(), bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("baton-newrelic: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("API-Key", c.apikey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("baton-newrelic: unexpected status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("baton-newrelic: failed to read response body: %w", err)
	}

	// Detect total failure: data=null with errors present. Partial-data responses
	// (data != null alongside errors) are tolerated on read paths.
	var raw struct {
		Data   json.RawMessage `json:"data"`
		Errors []GraphqlError  `json:"errors"`
	}
	if json.Unmarshal(bodyBytes, &raw) == nil && len(raw.Errors) > 0 {
		if len(raw.Data) == 0 || string(raw.Data) == "null" {
			return fmt.Errorf("baton-newrelic: graphql read failed: %s", raw.Errors[0].Message)
		}
	}

	if err := json.Unmarshal(bodyBytes, res); err != nil {
		return fmt.Errorf("baton-newrelic: failed to decode response body: %w", err)
	}
	return nil
}

// doRequest executes a GraphQL query and fails if the response contains any
// top-level errors. Use for mutations where a GraphQL error must surface as a
// Go error. For sync/list reads that can return partial data, use doReadRequest.
func (c *Client) doRequest(ctx context.Context, q string, v map[string]interface{}, res interface{}) error {
	body := &GraphqlBody{
		Query:     q,
		Variables: v,
	}
	reqBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("baton-newrelic: failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL.String(),
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return fmt.Errorf("baton-newrelic: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("API-Key", c.apikey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("baton-newrelic: unexpected status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("baton-newrelic: failed to read response body: %w", err)
	}

	var errCheck struct {
		Errors []GraphqlError `json:"errors"`
	}
	if json.Unmarshal(bodyBytes, &errCheck) == nil && len(errCheck.Errors) > 0 {
		return fmt.Errorf("baton-newrelic: graphql error: %s", errCheck.Errors[0].Message)
	}

	if err := json.Unmarshal(bodyBytes, res); err != nil {
		return fmt.Errorf("baton-newrelic: failed to decode response body: %w", err)
	}

	return nil
}
