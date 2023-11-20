package newrelic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	BaseHost        = "api.newrelic.com"
	GraphQHEndpoint = "/graphql"
)

type Client struct {
	httpClient *http.Client
	apikey     string
	baseURL    *url.URL
}

func NewClient(httpClient *http.Client, apikey string) *Client {
	u := &url.URL{
		Scheme: "https",
		Host:   BaseHost,
		Path:   GraphQHEndpoint,
	}

	return &Client{
		httpClient: httpClient,
		apikey:     apikey,
		baseURL:    u,
	}
}

func (c *Client) ListUsers(ctx context.Context, cursor string) ([]User, string, error) {
	var res UsersResponse

	err := c.Query(
		ctx,
		composeUsersQuery(),
		map[string]interface{}{"cursor": cursor},
		&res,
	)
	if err != nil {
		return nil, "", err
	}

	return res.Data.Actor.Users.Search.Users,
		res.Data.Actor.Users.Search.NextCursor,
		nil
}

func (c *Client) GetOrg(ctx context.Context) (*Org, error) {
	var res OrgDetailResponse

	err := c.Query(ctx, composeOrgQuery(), nil, &res)
	if err != nil {
		return nil, err
	}

	return &res.Data.Actor.Organization, nil
}

func (c *Client) ListRoles(ctx context.Context, cursor string) ([]Role, string, error) {
	var res RolesResponse

	err := c.Query(
		ctx,
		composeRolesQuery(),
		map[string]interface{}{"cursor": cursor},
		&res,
	)
	if err != nil {
		return nil, "", err
	}

	return res.Data.Actor.Organization.Management.Roles.Roles,
		res.Data.Actor.Organization.Management.Roles.NextCursor,
		nil
}

func parseCursor(cursor string) (string, string) {
	parts := strings.Split(cursor, "-")
	domainCursor, groupCursor := parts[0], parts[1]

	return domainCursor, groupCursor
}

func (c *Client) ListGroupsWithRole(ctx context.Context, roleId, cursor string) ([][]Group, string, []string, error) {
	var res GroupsResponse
	variables := map[string]interface{}{
		"roleId": roleId,
	}

	if cursor != "" {
		domainCursor, groupCursor := parseCursor(cursor)

		variables["domainCursor"] = domainCursor
		variables["groupCursor"] = groupCursor
	}

	err := c.Query(
		ctx,
		composeGroupsQuery(),
		variables,
		&res,
	)
	if err != nil {
		return nil, "", nil, err
	}

	var groupsCursors []string
	var groups [][]Group

	domains := res.Data.Actor.Organization.Management.Domains
	domainsCursor := domains.NextCursor
	for _, d := range domains.Domains {
		groups = append(groups, d.Groups.Groups)
		groupsCursors = append(groupsCursors, d.Groups.NextCursor)
	}

	return groups, domainsCursor, groupsCursors, nil
}

func (c *Client) ListGroups(ctx context.Context, cursor string) ([][]Group, string, []string, error) {
	var res GroupsResponse
	var variables map[string]interface{}

	if cursor != "" {
		domainCursor, groupCursor := parseCursor(cursor)

		variables = map[string]interface{}{
			"domainCursor": domainCursor,
			"groupCursor":  groupCursor,
		}
	}

	err := c.Query(
		ctx,
		composeGroupsQuery(),
		variables,
		&res,
	)
	if err != nil {
		return nil, "", nil, err
	}

	var groupsCursors []string
	var groups [][]Group

	domains := res.Data.Actor.Organization.Management.Domains
	domainsCursor := domains.NextCursor
	for _, d := range domains.Domains {
		groups = append(groups, d.Groups.Groups)
		groupsCursors = append(groupsCursors, d.Groups.NextCursor)
	}

	return groups, domainsCursor, groupsCursors, nil
}

func (c *Client) ListGroupMembers(ctx context.Context, groupId, domainCursor string) ([]string, string, error) {
	var res GroupMembersResponse

	err := c.Query(
		ctx,
		composeGroupMembersQuery(),
		map[string]interface{}{
			"groupId":      groupId,
			"domainCursor": domainCursor,
		},
		&res,
	)
	if err != nil {
		return nil, "", err
	}

	var users []string

	domains := res.Data.Actor.Organization.Management.Domains
	domainsCursor := domains.NextCursor

	// loop through domains if there is group with the same id
	for _, d := range domains.Domains {
		for _, g := range d.Groups.Groups {
			for _, u := range g.Users.Users {
				users = append(users, u.ID)
			}
		}
	}

	return users, domainsCursor, nil
}

type GraphqlBody struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

func (c *Client) Query(ctx context.Context, query string, variables map[string]interface{}, res interface{}) error {
	vars := make(map[string]interface{}, len(variables)+1)

	for k, v := range variables {
		vars[k] = v
	}

	vars["userId"] = c.apikey

	body := &GraphqlBody{
		Query:     query,
		Variables: vars,
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
