package newrelic

import "fmt"

// GraphQL queries and mutations.
const (
	actorBaseQ = "actor { %s }"

	usersQueryV2 = `organization {
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

	UsersQV2   = fmt.Sprintf(actorBaseQ, usersQueryV2)
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

// https://docs.newrelic.com/docs/apis/nerdgraph/examples/nerdgraph-manage-users/
func composeUsersQueryV2() string {
	return fmt.Sprintf(
		`query ListUsers($userCursor: String, $domainId: [ID!]) {
			%s
		}`, UsersQV2)
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

// composeGetUserByEmailQuery returns a NerdGraph query that finds a user by email
// within a single authentication domain and returns the v2 identity id required
// by user-management mutations (userManagementAddUsersToGroups, etc.).
func composeGetUserByEmailQuery() string {
	return `query GetUserByEmail($domainId: [ID!], $email: String!) {
		actor {
			organization {
				userManagement {
					authenticationDomains(id: $domainId) {
						authenticationDomains {
							users(filter: {email: {eq: $email}}) {
								users {
									id
									email
									name
								}
							}
						}
					}
				}
			}
		}
	}`
}

// composeGetUserByIDQuery returns a NerdGraph query that looks up a user by id across
// all authentication domains in the org (domainId is intentionally omitted). Filtering
// on a nonexistent id returns an empty users[] list with no errors, which is what
// makes this usable as an existence check ahead of DeleteUser. Paginates
// authenticationDomains itself via $domainCursor, the same connection ListAllDomains
// follows, so an org with more domains than fit on one page isn't scanned partially.
func composeGetUserByIDQuery() string {
	return `query GetUserByID($userId: ID!, $domainCursor: String) {
		actor {
			organization {
				userManagement {
					authenticationDomains(cursor: $domainCursor) {
						nextCursor
						authenticationDomains {
							users(filter: {id: {eq: $userId}}) {
								users {
									id
									email
									name
								}
							}
						}
					}
				}
			}
		}
	}`
}

func composeCreateUserMutation() string {
	return `mutation CreateUser($authDomainId: ID!, $email: String!, $name: String!, $userType: UserManagementRequestedTierName!) {
		userManagementCreateUser(
			createUserOptions: {
				authenticationDomainId: $authDomainId
				email: $email
				name: $name
				userType: $userType
			}
		) {
			createdUser {
				id
				email
				name
				type {
					displayName
				}
			}
		}
	}`
}

// composeUpdateUserMutation builds a mutation that only includes fields the caller
// is actually setting. Omitting a field entirely prevents NerdGraph from nulling it.
func composeUpdateUserMutation(updateFields []string) string {
	varDecl := "$userId: ID!"
	optBody := ""
	for _, f := range updateFields {
		switch f {
		case emailKey:
			varDecl += ", $email: String"
			optBody += "\n\t\t\t\temail: $email"
		case nameKey:
			varDecl += ", $name: String"
			optBody += "\n\t\t\t\tname: $name"
		case userTypeKey:
			varDecl += ", $userType: UserManagementRequestedTierName"
			optBody += "\n\t\t\t\tuserType: $userType"
		}
	}
	return fmt.Sprintf(`mutation UpdateUser(%s) {
		userManagementUpdateUser(
			updateUserOptions: {
				id: $userId%s
			}
		) {
			user {
				id
				email
				name
			}
		}
	}`, varDecl, optBody)
}

func composeDeleteUserMutation() string {
	return `mutation DeleteUser($userId: ID!) {
		userManagementDeleteUser(
			deleteUserOptions: {
				id: $userId
			}
		) {
			deletedUser {
				id
			}
		}
	}`
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

type UsersResponseV2 = QueryResponse[struct {
	Organization struct {
		UserManagement struct {
			AuthenticationDomains struct {
				AuthenticationDomains []struct {
					Users struct {
						ListBase
						Users []UserV2 `json:"users"`
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
	GraphqlErrorResponse
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
	GraphqlErrorResponse
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
	GraphqlErrorResponse
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
	GraphqlErrorResponse
	Data struct {
		MutData struct {
			Roles []struct {
				DisplayName string `json:"displayName"`
				ID          int    `json:"roleId"`
			} `json:"roles"`
		} `json:"authorizationManagementRevokeAccess"`
	} `json:"data"`
}

// GraphqlError represents a single error in a NerdGraph response.
type GraphqlError struct {
	Message    string                 `json:"message"`
	Extensions map[string]interface{} `json:"extensions"`
}

// GraphqlErrorResponse wraps data with possible top-level errors.
type GraphqlErrorResponse struct {
	Errors []GraphqlError `json:"errors"`
}

type CreateUserResponse struct {
	GraphqlErrorResponse
	Data struct {
		UserManagementCreateUser struct {
			CreatedUser struct {
				ID    string `json:"id"`
				Email string `json:"email"`
				Name  string `json:"name"`
				Type  struct {
					DisplayName string `json:"displayName"`
				} `json:"type"`
			} `json:"createdUser"`
		} `json:"userManagementCreateUser"`
	} `json:"data"`
}

type UpdateUserResponse struct {
	GraphqlErrorResponse
	Data struct {
		UserManagementUpdateUser struct {
			User struct {
				ID    string `json:"id"`
				Email string `json:"email"`
				Name  string `json:"name"`
			} `json:"user"`
		} `json:"userManagementUpdateUser"`
	} `json:"data"`
}

type DeleteUserResponse struct {
	GraphqlErrorResponse
	Data struct {
		UserManagementDeleteUser struct {
			DeletedUser struct {
				ID string `json:"id"`
			} `json:"deletedUser"`
		} `json:"userManagementDeleteUser"`
	} `json:"data"`
}

// GetUserByEmailResponse holds the result of a composeGetUserByEmailQuery call.
type GetUserByEmailResponse struct {
	GraphqlErrorResponse
	Data struct {
		Actor struct {
			Organization struct {
				UserManagement struct {
					AuthenticationDomains struct {
						AuthenticationDomains []struct {
							Users struct {
								Users []UserV2 `json:"users"`
							} `json:"users"`
						} `json:"authenticationDomains"`
					} `json:"authenticationDomains"`
				} `json:"userManagement"`
			} `json:"organization"`
		} `json:"actor"`
	} `json:"data"`
}

// GetUserByIDResponse holds the result of a composeGetUserByIDQuery call.
type GetUserByIDResponse struct {
	GraphqlErrorResponse
	Data struct {
		Actor struct {
			Organization struct {
				UserManagement struct {
					AuthenticationDomains struct {
						NextCursor            string `json:"nextCursor"`
						AuthenticationDomains []struct {
							Users struct {
								Users []UserV2 `json:"users"`
							} `json:"users"`
						} `json:"authenticationDomains"`
					} `json:"authenticationDomains"`
				} `json:"userManagement"`
			} `json:"organization"`
		} `json:"actor"`
	} `json:"data"`
}
