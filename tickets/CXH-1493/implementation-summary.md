# CXH-1493 — Implementation Summary

## What Was Built

### Overview
Added full user-account lifecycle provisioning to baton-newrelic:
- **CreateAccount** — three-step flow (exists-check → create → group-add), returns `AlreadyExistsResult` for duplicates
- **Delete** — permanent deletion via `userManagementDeleteUser`; not-found returns success (idempotent)
- **update_user action** — GlobalAction for profile updates (name, email, userType) via `userManagementUpdateUser`
- **Test server** — mock NerdGraph endpoint at `cmd/test-server/`
- **New config field** — `authentication-domain-id` (optional, provides default for account creation)

---

## Files Changed / Created

### Modified
| File | What Changed |
|------|-------------|
| `pkg/newrelic/graphql.go` | Added `composeCreateUserMutation`, `composeUpdateUserMutation`, `composeDeleteUserMutation`, and response types `CreateUserResponse`, `UpdateUserResponse`, `DeleteUserResponse`, `GraphqlError`, `GraphqlErrorResponse` |
| `pkg/newrelic/client.go` | Added `GetUserByEmail`, `CreateUser`, `UpdateUser`, `DeleteUser`, helper `doRawRequest`; added `userIDKey` constant; added `strings` import |
| `pkg/connector/users.go` | Added `CreateAccount`, `CreateAccountCapabilityDetails`, `Delete`; updated `newUserBuilder` to accept `authDomainID`; added `profileEmail` / `profileUserID` constants |
| `pkg/connector/connector.go` | Added `authenticationDomainID` field to `NewRelic` struct; updated `Metadata()` to include `AccountCreationSchema`; added `GlobalActions()` implementing `update_user` action; updated `New()` signature and `ResourceSyncers()` |
| `pkg/config/config.go` | Added `AuthenticationDomainIDField` |
| `pkg/config/conf.gen.go` | Regenerated — added `AuthenticationDomainId string` |
| `cmd/baton-newrelic/main.go` | Updated `getConnector` to pass `cc.AuthenticationDomainId` to `connector.New()` |
| `baton_capabilities.json` | Regenerated from built binary — added `CAPABILITY_ACCOUNT_PROVISIONING`, `CAPABILITY_RESOURCE_DELETE` for user; `CAPABILITY_ACTIONS` at connector level |

### Created
| File | What It Does |
|------|-------------|
| `cmd/test-server/main.go` | Mock NerdGraph server with in-memory state; handles all sync queries + create/update/delete/addToGroup/removeFromGroup mutations; seed user `alice@example.com`; duplicate-email detection |
| `config_schema.json` | Generated from binary — full connector config schema |
| `tickets/CXH-1493/ticket-brief.md` | Ticket intent + API contract |
| `tickets/CXH-1493/implementation-summary.md` | This file |

---

## New Config Field

| Field | CLI flag | Env var | Required |
|-------|----------|---------|----------|
| Authentication Domain ID | `--authentication-domain-id` | `BATON_AUTHENTICATION_DOMAIN_ID` | No (optional default for account provisioning) |

If not set in config, the `authentication_domain_id` must be supplied per-account-creation via the AccountCreationSchema form field.

---

## AccountCreationSchema Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | User's full display name |
| `email` | Yes | Email (used as login) |
| `user_type` | Yes | `BASIC_USER_TIER`, `CORE_USER_TIER`, or `FULL_USER_TIER` |
| `authentication_domain_id` | Yes | Auth domain ID (pre-populated from config if set) |
| `group_id` | No | Group to add the user to immediately after creation |

---

## CreateAccount Flow (three steps)

1. **Exists check** — `GetUserByEmail` walks paginated `ListUsers`; if found → `AlreadyExistsResult`
2. **Create** — `userManagementCreateUser` mutation with `authenticationDomainId`, `email`, `name`, `userType`
3. **Group-add** — if `group_id` is present in profile, calls `userManagementAddUsersToGroups`

---

## baton-test Results (mock path — localhost:18080)

```
baton-test run sync                                → PASS (4 resources, 2 entitlements, 1 grant)
baton-test run provisioning user --full            → PASS (create user-101, verify, delete, dup-delete, verify gone)
baton-test run provisioning user (duplicate email) → PASS (AlreadyExistsResult detected, reported as ⚠ already exists)
baton-test run action update_user                  → PASS (name update invoked + completed)
```

**Limitation:** Live verification against a real New Relic tenant is deferred — no non-prod credentials are available in this environment. All tests run against `cmd/test-server` (mock path).

---

## Capabilities After This Change

```json
{
  "user": ["CAPABILITY_SYNC", "CAPABILITY_ACCOUNT_PROVISIONING", "CAPABILITY_RESOURCE_DELETE"],
  "connectorCapabilities": ["CAPABILITY_PROVISION", "CAPABILITY_SYNC", "CAPABILITY_ACCOUNT_PROVISIONING", "CAPABILITY_RESOURCE_DELETE", "CAPABILITY_ACTIONS"]
}
```
