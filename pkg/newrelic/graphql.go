package newrelic

import "fmt"

// TODO: add more comments

const (
	baseQ      = "requestContext { userId }"
	actorBaseQ = "actor { %s }"

	usersQuery = `users {
		userSearch(cursor: $userCursor) { 
			nextCursor 
			totalCount 
			users { 
				email 
				name 
				userId 
			} 
		}
	 }`

	orgQuery = `organization {
		id
		name
	}`

	rolesQuery = `organization {
		authorizationManagement {
			roles(cursor: $roleCursor) {
				nextCursor
				totalCount
				roles {
					id
					displayName
					name
					scope
				}
			}
		}
	}`

	groupsQuery = `organization {
		authorizationManagement {
			authenticationDomains(id: $domainId) {
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
			}
		}
	}`

	groupRolesQuery = `organization {
		authorizationManagement {
			authenticationDomains(id: $domainId) {
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
			}
		}
	}`

	domainsQuery = `organization {
		authorizationManagement {
			authenticationDomains(cursor: $cursor) {
				nextCursor
				totalCount
				authenticationDomains {
					id
					name
					groups {
						totalCount
					}
				}
			}
		}
	}`

	groupMembersQuery = `organization {
		userManagement {
		  authenticationDomains(id: $domainId) {
			authenticationDomains {
			  groups(id: $groupId) {
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
				nextCursor
				totalCount
			  }
			}
		  }
		}
	  }`
)

var (
	UsersQ        = fmt.Sprintf(actorBaseQ, usersQuery)
	OrgQ          = fmt.Sprintf(actorBaseQ, orgQuery)
	RolesQ        = fmt.Sprintf(actorBaseQ, rolesQuery)
	GroupsQ       = fmt.Sprintf(actorBaseQ, groupsQuery)
	GroupRolesQ   = fmt.Sprintf(actorBaseQ, groupRolesQuery)
	DomainsQ      = fmt.Sprintf(actorBaseQ, domainsQuery)
	GroupMembersQ = fmt.Sprintf(actorBaseQ, groupMembersQuery)
)

func composeUsersQuery() string {
	return fmt.Sprintf(
		`query ListUsers($userCursor: String) {
			%s
			%s
		}`, baseQ, UsersQ)
}

func composeOrgQuery() string {
	return fmt.Sprintf(
		`query GetOrg {
			%s
			%s
		}`, baseQ, OrgQ)
}

func composeRolesQuery() string {
	return fmt.Sprintf(
		`query ListRoles($roleCursor: String) {
			%s
			%s
		}`, baseQ, RolesQ)
}

func composeDomainsQuery() string {
	return fmt.Sprintf(
		`query ListDomains($cursor: String) {
			%s
			%s
		}`, baseQ, DomainsQ)
}

func composeGroupsQuery() string {
	return fmt.Sprintf(
		`query ListGroups($domainId: [ID!], $groupCursor: String) {
			%s
			%s
		}`, baseQ, GroupsQ)
}

func composeAllGroupsWithRoleQuery() string {
	return fmt.Sprintf(
		`query ListGroups($domainId: [ID!], $roleId: [ID!], $groupCursor: String) {
			%s
			%s
		}`, baseQ, GroupRolesQ)
}

func composeGroupMembersQuery() string {
	return fmt.Sprintf(
		`query ListGroupMembers($domainId: [ID!], $groupId: [ID!], $membersCursor: String) {
			%s
			%s
		}`, baseQ, GroupMembersQ)
}

type QueryResponse[T any] struct {
	Data struct {
		Actor T `json:"actor"`
	} `json:"data"`
}

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
				Users []struct {
					ID string `json:"id"`
				} `json:"users"`
			} `json:"users"`
		} `json:"groups"`
	} `json:"groups"`
}]
