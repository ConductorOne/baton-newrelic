// Package main implements a mock NerdGraph server for integration testing.
// It handles the user-management GraphQL mutations (create, update, delete)
// and the queries needed for baton-test sync, returning deterministic seed data.
//
// Usage:
//
//	go run ./cmd/test-server --addr :18080
//	baton-newrelic --base-url http://localhost:18080 --apikey dummy
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ---- JSON key constants ----

const (
	keyActor        = "actor"
	keyAuthDomains  = "authenticationDomains"
	keyAuthMgmt     = "authorizationManagement"
	keyDisplayName  = "displayName"
	keyEmail        = "email"
	keyGroups       = "groups"
	keyID           = "id"
	keyName         = "name"
	keyNextCursor   = "nextCursor"
	keyOrganization = "organization"
	keyRoles        = "roles"
	keyTotalCount   = "totalCount"
	keyType         = "type"
	keyUserID       = "userId"
	keyUserMgmt     = "userManagement"
	keyUsers        = "users"
)

// ---- seed data ----

const (
	seedOrgID      = "org-001"
	seedOrgName    = "Test Organization"
	seedDomainID   = "domain-001"
	seedDomainName = "Default Domain"
	seedGroupID    = "group-001"
	seedGroupName  = "Admins"
	seedRoleID     = "role-001"
	seedRoleName   = "Organization manager"
	seedUserID     = "user-001"
	seedUserEmail  = "alice@example.com"
	seedUserName   = "Alice Example"
)

// ---- in-memory store ----

type userRecord struct {
	ID       string
	Email    string
	Name     string
	UserType string
}

type groupRecord struct {
	ID      string
	Name    string
	Members map[string]bool // userID → true
}

type store struct {
	mu     sync.Mutex
	users  map[string]*userRecord
	groups map[string]*groupRecord
	nextID int
}

func newStore() *store {
	return &store{
		users: map[string]*userRecord{
			seedUserID: {ID: seedUserID, Email: seedUserEmail, Name: seedUserName, UserType: "FULL_USER_TIER"},
		},
		groups: map[string]*groupRecord{
			seedGroupID: {ID: seedGroupID, Name: seedGroupName, Members: map[string]bool{}},
		},
		nextID: 100,
	}
}

func (s *store) newID(prefix string) string {
	s.nextID++
	return fmt.Sprintf("%s-%d", prefix, s.nextID)
}

// ---- GraphQL request/response helpers ----

type gqlRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type gqlError struct {
	Message string `json:"message"`
}

func gqlOK(data interface{}) interface{} {
	return map[string]interface{}{"data": data}
}

func gqlErr(msg string) interface{} {
	return map[string]interface{}{
		"errors": []gqlError{{Message: msg}},
		"data":   nil,
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ---- mutation handlers ----

func (s *store) handleCreateUser(vars map[string]interface{}) interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	email, _ := vars[keyEmail].(string)
	name, _ := vars[keyName].(string)
	userType, _ := vars["userType"].(string)

	for _, u := range s.users {
		if u.Email == email {
			return gqlErr(fmt.Sprintf("user with email %s already exists in this organization", email))
		}
	}

	id := s.newID("user")
	if userType == "" {
		userType = "FULL_USER_TIER"
	}
	s.users[id] = &userRecord{ID: id, Email: email, Name: name, UserType: userType}

	return gqlOK(map[string]interface{}{
		"userManagementCreateUser": map[string]interface{}{
			"createdUser": map[string]interface{}{
				keyID:    id,
				keyEmail: email,
				keyName:  name,
				keyType:  map[string]interface{}{keyDisplayName: userType},
			},
		},
	})
}

func (s *store) handleUpdateUser(vars map[string]interface{}) interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, _ := vars[keyUserID].(string)
	u, ok := s.users[id]
	if !ok {
		return gqlErr(fmt.Sprintf("user %s not found", id))
	}

	if v, ok2 := vars[keyEmail].(string); ok2 && v != "" {
		u.Email = v
	}
	if v, ok2 := vars[keyName].(string); ok2 && v != "" {
		u.Name = v
	}
	if v, ok2 := vars["userType"].(string); ok2 && v != "" {
		u.UserType = v
	}

	return gqlOK(map[string]interface{}{
		"userManagementUpdateUser": map[string]interface{}{
			"user": map[string]interface{}{
				keyID:    u.ID,
				keyEmail: u.Email,
				keyName:  u.Name,
			},
		},
	})
}

func (s *store) handleDeleteUser(vars map[string]interface{}) interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, _ := vars[keyUserID].(string)
	delete(s.users, id)
	for _, g := range s.groups {
		delete(g.Members, id)
	}

	return gqlOK(map[string]interface{}{
		"userManagementDeleteUser": map[string]interface{}{
			"deletedUser": map[string]interface{}{keyID: id},
		},
	})
}

func (s *store) handleAddUsersToGroups(vars map[string]interface{}) interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupID, _ := vars["groupId"].(string)
	userID, _ := vars[keyUserID].(string)

	g, ok := s.groups[groupID]
	if !ok {
		return gqlErr(fmt.Sprintf("group %s not found", groupID))
	}
	g.Members[userID] = true

	return gqlOK(map[string]interface{}{
		"userManagementAddUsersToGroups": map[string]interface{}{
			keyGroups: []map[string]interface{}{
				{keyID: groupID, keyDisplayName: g.Name},
			},
		},
	})
}

func (s *store) handleRemoveUsersFromGroups(vars map[string]interface{}) interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupID, _ := vars["groupId"].(string)
	userID, _ := vars[keyUserID].(string)

	if g, ok := s.groups[groupID]; ok {
		delete(g.Members, userID)
	}

	return gqlOK(map[string]interface{}{
		"userManagementRemoveUsersFromGroups": map[string]interface{}{
			keyGroups: []map[string]interface{}{
				{keyID: groupID, keyDisplayName: ""},
			},
		},
	})
}

// ---- query handlers ----

func handleAccountsQuery() interface{} {
	return gqlOK(map[string]interface{}{
		keyActor: map[string]interface{}{
			"accounts": []map[string]interface{}{
				{keyID: 12345},
			},
		},
	})
}

func handleOrgQuery() interface{} {
	return gqlOK(map[string]interface{}{
		keyActor: map[string]interface{}{
			keyOrganization: map[string]interface{}{
				keyID:   seedOrgID,
				keyName: seedOrgName,
			},
		},
	})
}

func (s *store) handleUsersQuery() interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	userList := make([]map[string]interface{}, 0, len(s.users))
	for _, u := range s.users {
		userList = append(userList, map[string]interface{}{
			keyUserID: u.ID,
			keyEmail:  u.Email,
			keyName:   u.Name,
		})
	}

	return gqlOK(map[string]interface{}{
		keyActor: map[string]interface{}{
			keyUsers: map[string]interface{}{
				"userSearch": map[string]interface{}{
					keyUsers:      userList,
					keyNextCursor: nil,
					keyTotalCount: len(userList),
				},
			},
		},
	})
}

func (s *store) handleUsersQueryV2() interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	userList := make([]map[string]interface{}, 0, len(s.users))
	for _, u := range s.users {
		userList = append(userList, map[string]interface{}{
			keyID:                    u.ID,
			keyEmail:                 u.Email,
			keyName:                  u.Name,
			"emailVerificationState": "VERIFIED",
		})
	}

	return gqlOK(map[string]interface{}{
		keyActor: map[string]interface{}{
			keyOrganization: map[string]interface{}{
				keyUserMgmt: map[string]interface{}{
					keyAuthDomains: map[string]interface{}{
						keyAuthDomains: []map[string]interface{}{
							{
								keyUsers: map[string]interface{}{
									keyUsers:      userList,
									keyNextCursor: nil,
									keyTotalCount: len(userList),
								},
							},
						},
					},
				},
			},
		},
	})
}

func (s *store) handleGetUserByEmailQuery(vars map[string]interface{}) interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	email, _ := vars[keyEmail].(string)

	userList := make([]map[string]interface{}, 0, 1)
	for _, u := range s.users {
		if strings.EqualFold(u.Email, email) {
			userList = append(userList, map[string]interface{}{
				keyID:    u.ID,
				keyEmail: u.Email,
				keyName:  u.Name,
			})
			break
		}
	}

	return gqlOK(map[string]interface{}{
		keyActor: map[string]interface{}{
			keyOrganization: map[string]interface{}{
				keyUserMgmt: map[string]interface{}{
					keyAuthDomains: map[string]interface{}{
						keyAuthDomains: []map[string]interface{}{
							{
								keyUsers: map[string]interface{}{
									keyUsers: userList,
								},
							},
						},
					},
				},
			},
		},
	})
}

func (s *store) handleDomainsQuery() interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	return gqlOK(map[string]interface{}{
		keyActor: map[string]interface{}{
			keyOrganization: map[string]interface{}{
				keyAuthMgmt: map[string]interface{}{
					keyAuthDomains: map[string]interface{}{
						keyNextCursor: nil,
						keyTotalCount: 1,
						keyAuthDomains: []map[string]interface{}{
							{
								keyID:   seedDomainID,
								keyName: seedDomainName,
								keyGroups: map[string]interface{}{
									keyTotalCount: len(s.groups),
								},
							},
						},
					},
				},
			},
		},
	})
}

func (s *store) handleGroupsQuery() interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupList := make([]map[string]interface{}, 0, len(s.groups))
	for _, g := range s.groups {
		groupList = append(groupList, map[string]interface{}{
			keyID:          g.ID,
			keyDisplayName: g.Name,
			keyRoles:       map[string]interface{}{keyTotalCount: 1},
		})
	}

	return gqlOK(map[string]interface{}{
		keyActor: map[string]interface{}{
			keyOrganization: map[string]interface{}{
				keyAuthMgmt: map[string]interface{}{
					keyAuthDomains: map[string]interface{}{
						keyNextCursor: nil,
						keyTotalCount: 1,
						keyAuthDomains: []map[string]interface{}{
							{
								keyID:   seedDomainID,
								keyName: seedDomainName,
								keyGroups: map[string]interface{}{
									keyNextCursor: nil,
									keyTotalCount: len(groupList),
									keyGroups:     groupList,
								},
							},
						},
					},
				},
			},
		},
	})
}

func (s *store) handleGroupMembersQuery(vars map[string]interface{}) interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupID, _ := vars["groupId"].(string)

	g, ok := s.groups[groupID]
	if !ok {
		return gqlErr(fmt.Sprintf("group %s not found", groupID))
	}

	memberList := make([]map[string]interface{}, 0, len(g.Members))
	for uid := range g.Members {
		memberList = append(memberList, map[string]interface{}{keyID: uid})
	}

	return gqlOK(map[string]interface{}{
		keyActor: map[string]interface{}{
			keyOrganization: map[string]interface{}{
				keyUserMgmt: map[string]interface{}{
					keyAuthDomains: map[string]interface{}{
						keyAuthDomains: []map[string]interface{}{
							{
								keyGroups: map[string]interface{}{
									keyNextCursor: nil,
									keyTotalCount: 1,
									keyGroups: []map[string]interface{}{
										{
											keyID:          g.ID,
											keyDisplayName: g.Name,
											keyUsers: map[string]interface{}{
												keyNextCursor: nil,
												keyTotalCount: len(memberList),
												keyUsers:      memberList,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
}

func handleRolesQuery() interface{} {
	return gqlOK(map[string]interface{}{
		keyActor: map[string]interface{}{
			keyOrganization: map[string]interface{}{
				keyAuthMgmt: map[string]interface{}{
					keyRoles: map[string]interface{}{
						keyNextCursor: nil,
						keyTotalCount: 1,
						keyRoles: []map[string]interface{}{
							{
								keyID:          seedRoleID,
								keyName:        "organization_manager",
								keyDisplayName: seedRoleName,
								"scope":        "organization",
							},
						},
					},
				},
			},
		},
	})
}

func (s *store) handleGroupsWithRoleQuery() interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupList := make([]map[string]interface{}, 0, len(s.groups))
	for _, g := range s.groups {
		groupList = append(groupList, map[string]interface{}{
			keyID:          g.ID,
			keyDisplayName: g.Name,
			keyRoles: map[string]interface{}{
				keyNextCursor: nil,
				keyTotalCount: 1,
				keyRoles: []map[string]interface{}{
					{keyID: seedRoleID, keyName: "organization_manager", keyDisplayName: seedRoleName},
				},
			},
		})
	}

	return gqlOK(map[string]interface{}{
		keyActor: map[string]interface{}{
			keyOrganization: map[string]interface{}{
				keyAuthMgmt: map[string]interface{}{
					keyAuthDomains: map[string]interface{}{
						keyNextCursor: nil,
						keyTotalCount: 1,
						keyAuthDomains: []map[string]interface{}{
							{
								keyID:   seedDomainID,
								keyName: seedDomainName,
								keyGroups: map[string]interface{}{
									keyNextCursor: nil,
									keyTotalCount: len(groupList),
									keyGroups:     groupList,
								},
							},
						},
					},
				},
			},
		},
	})
}

func handleGrantAccess() interface{} {
	return gqlOK(map[string]interface{}{
		"authorizationManagementGrantAccess": map[string]interface{}{
			keyRoles: []map[string]interface{}{
				{"roleId": 1, keyDisplayName: seedRoleName},
			},
		},
	})
}

func handleRevokeAccess() interface{} {
	return gqlOK(map[string]interface{}{
		"authorizationManagementRevokeAccess": map[string]interface{}{
			keyRoles: []map[string]interface{}{
				{"roleId": 1, keyDisplayName: seedRoleName},
			},
		},
	})
}

// ---- dispatch ----

func (s *store) dispatch(req gqlRequest) interface{} {
	q := req.Query
	vars := req.Variables

	switch {
	// mutations (more specific first)
	case strings.Contains(q, "userManagementCreateUser"):
		return s.handleCreateUser(vars)
	case strings.Contains(q, "userManagementUpdateUser"):
		return s.handleUpdateUser(vars)
	case strings.Contains(q, "userManagementDeleteUser"):
		return s.handleDeleteUser(vars)
	case strings.Contains(q, "userManagementAddUsersToGroups"):
		return s.handleAddUsersToGroups(vars)
	case strings.Contains(q, "userManagementRemoveUsersFromGroups"):
		return s.handleRemoveUsersFromGroups(vars)
	case strings.Contains(q, "authorizationManagementGrantAccess"):
		return handleGrantAccess()
	case strings.Contains(q, "authorizationManagementRevokeAccess"):
		return handleRevokeAccess()
	// queries
	case strings.Contains(q, "accounts"):
		return handleAccountsQuery()
	case strings.Contains(q, "ListUsers") && strings.Contains(q, "userManagement"):
		return s.handleUsersQueryV2()
	case strings.Contains(q, "ListUsers"):
		return s.handleUsersQuery()
	case strings.Contains(q, "GetOrg"):
		return handleOrgQuery()
	case strings.Contains(q, "GetUserByEmail"):
		return s.handleGetUserByEmailQuery(vars)
	case strings.Contains(q, "ListRoles"):
		return handleRolesQuery()
	case strings.Contains(q, "ListDomains"):
		return s.handleDomainsQuery()
	case strings.Contains(q, "ListGroupMembers"):
		return s.handleGroupMembersQuery(vars)
	case strings.Contains(q, "ListGroups") && strings.Contains(q, "roleId"):
		return s.handleGroupsWithRoleQuery()
	case strings.Contains(q, "ListGroups"):
		return s.handleGroupsQuery()
	default:
		n := len(q)
		if n > 120 {
			n = 120
		}
		return gqlErr("test-server: unrecognised query/mutation: " + q[:n])
	}
}

// ---- HTTP handler ----

func makeHandler(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gqlErr("only POST is supported"))
			return
		}

		var req gqlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gqlErr("invalid JSON body: "+err.Error()))
			return
		}

		writeJSON(w, http.StatusOK, s.dispatch(req))
	}
}

func run() error {
	addr := flag.String("addr", ":18080", "listen address")
	flag.Parse()

	s := newStore()
	h := makeHandler(s)

	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", h)
	mux.HandleFunc("/", h)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	_, _ = fmt.Fprintln(os.Stderr, "test-server listening on", *addr)
	return srv.ListenAndServe()
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "test-server error:", err)
		os.Exit(1)
	}
}
