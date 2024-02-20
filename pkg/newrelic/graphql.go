package newrelic

import "fmt"

// GraphQL queries and mutations.
const (
	actorBaseQ = "actor { %s }"

	usersQuery = `organization {
		userManagement {
			authenticationDomains(id: $domainId) {
			authenticationDomains {
			  users(cursor: $userCursor) {
				users {
				  email
				  id
				  name
				  emailVerificationState
				}
				nextCursor
				totalCount
			  }
			}
		  }
		}
	  }`

	accountsQuery = `accounts {
		id
	}`

	orgQuery = `organization { %s }`

	managementQuery = `organization { authorizationManagement { %s } }`

	orgDetailQuery = `organization { 
		id
		name
	}`

	rolesQuery = `roles(cursor: $roleCursor) {
		nextCursor
		totalCount
		roles {
			id
			displayName
			name
			scope
		}
	}`

	groupsQuery = `authenticationDomains(id: $domainId) {
		authenticationDomains {
			id
			name
			groups(cursor: $groupCursor) {
				nextCursor
				totalCount
				groups {
					id
					displayName
					roles {
						totalCount
					}
				}
			}
		}
	}`

	groupRolesQuery = `authenticationDomains(id: $domainId) {
		nextCursor
		totalCount
		authenticationDomains {
			id
			name
			groups(cursor: $groupCursor) {
				nextCursor
				totalCount
				groups {
					id
					displayName
					roles(roleId: $roleId) {
						nextCursor
						totalCount
						roles {
							id
							name
							displayName
						}
					}
				}
			}
		}
	}`

	domainsQuery = `authenticationDomains(cursor: $cursor) {
		nextCursor
		totalCount
		authenticationDomains {
			id
			name
			groups {
				totalCount
			}
		}
	}`

	groupMembersQuery = `userManagement {
		authenticationDomains(id: $domainId) {
			authenticationDomains {
				groups(id: $groupId) {
					nextCursor
					totalCount
					groups {
						id
						displayName
						users(cursor: $membersCursor) {
							nextCursor
							totalCount
							users {
								id
							}
						}
					}
				}
			}
		}
	}`

	addGroupMemberMutation = `userManagementAddUsersToGroups(
		addUsersToGroupsOptions: {
			groupIds: [$groupId]
			userIds: [$userId]
		}
	) {
		groups {
			displayName
			id
		}
	}`

	removeGroupMemberMutation = `userManagementRemoveUsersFromGroups(
		removeUsersFromGroupsOptions: {
			groupIds: [$groupId]
			userIds: [$userId]
		}
	) {
		groups {
			displayName
			id
		}
	}`

	addRoleMutation = `authorizationManagementGrantAccess(
		grantAccessOptions: {
			groupId: $groupId 
		 	%s
		}
	) {
		roles {
			displayName
			roleId
		}
	}`

	groupAccessGrants = `groupAccessGrants: {
		groupId: $groupId
		roleId: $roleId
	}`

	accountAccessGrants = `accountAccessGrants: {
		accountId: $accountId
		roleId: $roleId
	}`

	orgAccessGrants = `organizationAccessGrants: {
		roleId: $roleId
	}`

	removeRoleMutation = `authorizationManagementRevokeAccess(
		revokeAccessOptions: {
			groupId: $groupId
			%s
		}
	) {
		roles {
			displayName
			roleId
		}
	}`
)

var (
	ManagementsQ = fmt.Sprintf(actorBaseQ, managementQuery)
	OrgQ         = fmt.Sprintf(actorBaseQ, orgQuery)
	AccountsQ    = fmt.Sprintf(actorBaseQ, accountsQuery)

	UsersQ     = fmt.Sprintf(actorBaseQ, usersQuery)
	OrgDetailQ = fmt.Sprintf(actorBaseQ, orgDetailQuery)

	RolesQ        = fmt.Sprintf(ManagementsQ, rolesQuery)
	GroupsQ       = fmt.Sprintf(ManagementsQ, groupsQuery)
	GroupRolesQ   = fmt.Sprintf(ManagementsQ, groupRolesQuery)
	DomainsQ      = fmt.Sprintf(ManagementsQ, domainsQuery)
	GroupMembersQ = fmt.Sprintf(OrgQ, groupMembersQuery)

	AddGroupRole   = fmt.Sprintf(addRoleMutation, groupAccessGrants)
	AddAccountRole = fmt.Sprintf(addRoleMutation, accountAccessGrants)
	AddOrgRole     = fmt.Sprintf(addRoleMutation, orgAccessGrants)

	RemoveGroupRole   = fmt.Sprintf(removeRoleMutation, groupAccessGrants)
	RemoveAccountRole = fmt.Sprintf(removeRoleMutation, accountAccessGrants)
	RemoveOrgRole     = fmt.Sprintf(removeRoleMutation, orgAccessGrants)
)

func composeAccountsQuery() string {
	return fmt.Sprintf(
		`query ListAccounts {
			%s
		}`, AccountsQ)
}

func composeUsersQuery() string {
	return fmt.Sprintf(
		`query ListUsers($userCursor: String, $domainId: [ID!]) {
			%s
		}`, UsersQ)
}

func composeOrgQuery() string {
	return fmt.Sprintf(
		`query GetOrg {
			%s
		}`, OrgDetailQ)
}

func composeRolesQuery() string {
	return fmt.Sprintf(
		`query ListRoles($roleCursor: String) {
			%s
		}`, RolesQ)
}

func composeDomainsQuery() string {
	return fmt.Sprintf(
		`query ListDomains($cursor: String) {
			%s
		}`, DomainsQ)
}

func composeGroupsQuery() string {
	return fmt.Sprintf(
		`query ListGroups($domainId: [ID!], $groupCursor: String) {
			%s
		}`, GroupsQ)
}

func composeAllGroupsWithRoleQuery() string {
	return fmt.Sprintf(
		`query ListGroups($domainId: [ID!], $roleId: [ID!], $groupCursor: String) {
			%s
		}`, GroupRolesQ)
}

func composeGroupMembersQuery() string {
	return fmt.Sprintf(
		`query ListGroupMembers($domainId: [ID!], $groupId: [ID!], $membersCursor: String) {
			%s
		}`, GroupMembersQ)
}

func composeAddGroupMemberMutation() string {
	return fmt.Sprintf(
		`mutation AddGroupMember($groupId: ID!, $userId: ID!) {
			%s
		}`, addGroupMemberMutation)
}

func composeRemoveGroupMemberMutation() string {
	return fmt.Sprintf(
		`mutation RemoveGroupMember($groupId: ID!, $userId: ID!) {
			%s
		}`, removeGroupMemberMutation)
}

func composeAddGroupRoleMutation() string {
	return fmt.Sprintf(
		`mutation AddGroupRole($groupId: ID!, $roleId: ID!) {
			%s
		}`, AddGroupRole)
}

func composeAddAccountRoleMutation() string {
	return fmt.Sprintf(
		`mutation AddAccountRole($accountId: Int!, $groupId: ID!, $roleId: ID!) {
			%s
		}`, AddAccountRole)
}

func composeAddOrgRoleMutation() string {
	return fmt.Sprintf(
		`mutation AddOrgRole($groupId: ID!, $roleId: ID!) {
			%s
		}`, AddOrgRole)
}

func composeRemoveGroupRoleMutation() string {
	return fmt.Sprintf(
		`mutation RemoveGroupRole($groupId: ID!, $roleId: ID!) {
			%s
		}`, RemoveGroupRole)
}

func composeRemoveAccountRoleMutation() string {
	return fmt.Sprintf(
		`mutation RemoveAccountRole($accountId: Int!, $groupId: ID!, $roleId: ID!) {
			%s
		}`, RemoveAccountRole)
}

func composeRemoveOrgRoleMutation() string {
	return fmt.Sprintf(
		`mutation RemoveOrgRole($groupId: ID!, $roleId: ID!) {
			%s
		}`, RemoveOrgRole)
}

// Request body structure for graphql queries and mutations.
type GraphqlBody struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

// Response structures of graphql queries and mutations.
type QueryResponse[T any] struct {
	Data struct {
		Actor T `json:"actor"`
	} `json:"data"`
}

type AccountsResponse = QueryResponse[struct {
	Accounts []struct {
		ID int `json:"id"`
	} `json:"accounts"`
}]

type ListBase struct {
	NextCursor string `json:"nextCursor"`
	Total      int    `json:"totalCount"`
}

type UsersResponse = QueryResponse[struct {
	Users struct {
		Search struct {
			ListBase
			Users []User `json:"users"`
		} `json:"userSearch"`
	} `json:"users"`
}]

type UsersResponseV2 = QueryResponse[struct {
	Organization struct {
		UserManagement struct {
			AuthenticationDomains struct {
				AuthenticationDomains []struct {
					Users struct {
						ListBase
						Users []User `json:"users"`
					} `json:"users"`
				} `json:"authenticationDomains"`
			} `json:"authenticationDomains"`
		} `json:"userManagement"`
	} `json:"organization"`
}]

type OrgResponse[T any] QueryResponse[struct {
	Organization T `json:"organization"`
}]

type OrgDetailResponse = OrgResponse[Org]

type OrgAuthManagementResponse[T any] OrgResponse[struct {
	Management struct {
		Domains struct {
			ListBase
			Domains []T `json:"authenticationDomains"`
		} `json:"authenticationDomains"`
	} `json:"authorizationManagement"`
}]

type GroupsResponse = OrgAuthManagementResponse[struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Groups struct {
		ListBase
		Groups []Group `json:"groups"`
	} `json:"groups"`
}]

type RolesResponse = OrgResponse[struct {
	Management struct {
		Roles struct {
			ListBase
			Roles []Role `json:"roles"`
		} `json:"roles"`
	} `json:"authorizationManagement"`
}]

type OrgUserManagementResponse[T any] OrgResponse[struct {
	Management struct {
		Domains struct {
			ListBase
			Domains []T `json:"authenticationDomains"`
		} `json:"authenticationDomains"`
	} `json:"userManagement"`
}]

type GroupMembersResponse = OrgUserManagementResponse[struct {
	Groups struct {
		Groups []struct {
			DisplayName string `json:"displayName"`
			ID          string `json:"id"`
			Users       struct {
				ListBase
				Users []struct {
					ID string `json:"id"`
				} `json:"users"`
			} `json:"users"`
		} `json:"groups"`
	} `json:"groups"`
}]

type AddGroupMemberResponse struct {
	Data struct {
		MutData struct {
			Groups []struct {
				DisplayName string `json:"displayName"`
				ID          string `json:"id"`
			} `json:"groups"`
		} `json:"userManagementAddUsersToGroups"`
	} `json:"data"`
}

type RemoveGroupMemberResponse struct {
	Data struct {
		MutData struct {
			Groups []struct {
				DisplayName string `json:"displayName"`
				ID          string `json:"id"`
			} `json:"groups"`
		} `json:"userManagementRemoveUsersFromGroups"`
	} `json:"data"`
}

type GrantRoleResponse struct {
	Data struct {
		MutData struct {
			Roles []struct {
				DisplayName string `json:"displayName"`
				ID          int    `json:"roleId"`
			} `json:"roles"`
		} `json:"authorizationManagementGrantAccess"`
	} `json:"data"`
}

type RevokeRoleResponse struct {
	Data struct {
		MutData struct {
			Roles []struct {
				DisplayName string `json:"displayName"`
				ID          int    `json:"roleId"`
			} `json:"roles"`
		} `json:"authorizationManagementRevokeAccess"`
	} `json:"data"`
}
