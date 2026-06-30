# CXH-1493 — New Relic: Add account provisioning and deprovisioning

## INTENT (immutable)

Add user-account lifecycle provisioning to baton-newrelic via NerdGraph user-management mutations: **CREATE**, **UPDATE**, **DELETE**. Validate with a test-server (mock path — no live creds). Scope is exactly users create/update/delete. **OUT OF SCOPE**: deactivate (NerdGraph has none), SCIM, authentication-domain creation.

---

## API CONTRACT

**Endpoint:** single `POST https://api.newrelic.com/graphql` (EU: `https://api.eu.newrelic.com/graphql`).  
**Auth header:** `Api-Key: <user-key>` (key needs user-management write at account scope).  
**Body:** `{ "query": "<mutation>" }`.  
All ops are GraphQL mutations.

### CREATE
```graphql
mutation CreateUser(
  $authDomainId: ID!
  $email: String!
  $name: String!
  $userType: UserManagementRequestedTier!
) {
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
      type { displayName }
    }
  }
}
```
`createUserOptions` fields: `authenticationDomainId`, `email`, `name`, `userType ∈ {BASIC_USER_TIER, CORE_USER_TIER, FULL_USER_TIER}`.

### UPDATE
```graphql
mutation UpdateUser(
  $userId: ID!
  $email: String
  $name: String
  $userType: UserManagementRequestedTier
) {
  userManagementUpdateUser(
    updateUserOptions: {
      id: $userId
      email: $email
      name: $name
      userType: $userType
    }
  ) {
    user { id email name }
  }
}
```

### DELETE (permanent, no soft-deactivate)
```graphql
mutation DeleteUser($userId: ID!) {
  userManagementDeleteUser(
    deleteUserOptions: { id: $userId }
  ) {
    deletedUser { id }
  }
}
```

### IMPORTANT BEHAVIOURS
- A newly-created user has **zero account visibility** until added to a group.  
  Group-add chains via existing `userManagementAddUsersToGroups` after create.
- `authenticationDomainId` is **required** for create.  
  Sourced from config field `authentication-domain-id` (default) or overridden per-creation via the AccountCreationSchema profile field.
- **DELETE is permanent** — no soft-deactivate exists in NerdGraph.
- **Delete not-found = success** (idempotent).
