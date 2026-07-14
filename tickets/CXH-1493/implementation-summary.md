# CXH-1493 — Implementation Summary

## What Was Built

### Overview
Added full user-account lifecycle provisioning to baton-newrelic:
- **CreateAccount** — two-step flow (exists-check → create), returns `AlreadyExistsResult` for duplicates
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

---

## CreateAccount Flow (two steps)

1. **Exists check** — `GetUserByEmail`; if found → `AlreadyExistsResult`
2. **Create** — `userManagementCreateUser` mutation with `authenticationDomainId`, `email`, `name`, `userType`

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

---

## Review-Fixes Pass (post-commit 03b871b)

Addressed six findings from the PR review bot:

| # | Severity | Finding | Fix | Location |
|---|----------|---------|-----|----------|
| 1 | SHOULD-FIX | **UpdateUser null-propagation** — static mutation always sends `email/name/userType` as variables; absent fields arrive as `null` and NerdGraph clears them | `composeUpdateUserMutation` now accepts `[]string` of field names and builds a dynamic mutation that only includes fields being set | `pkg/newrelic/graphql.go:371`, `pkg/newrelic/client.go:594-626` |
| 2 | SHOULD-FIX | **GraphQL errors-array not inspected in doRequest** — HTTP 200 with `{"errors":[...]}` silently treated as success, causing orphaned users on AddUserToGroup failure | `doRequest` now reads the body into `[]byte`, unmarshal-checks a `{Errors}` struct, and returns an error if non-empty before decoding into `res` | `pkg/newrelic/client.go:684-734` |
| 3 | SHOULD-FIX | **Dead code: marshalAndPost** — always returned `nil, error`; caller discarded the `[]byte` with `_ =` | Removed `marshalAndPost`; `CreateUser` calls `doRawRequest` directly | `pkg/newrelic/client.go:565-590` |
| 4 | NIT | **PII logging** — `update_user` action logged `email` + `name` at Info level | Moved `name`/`email` to `l.Debug`; only `user_id` + `user_type` remain at Info | `pkg/connector/connector.go:176-182` |
| 5 | NIT | **Missing `baton-newrelic:` prefix** — marshal/request-create errors in `doRequest`/`doRawRequest` lacked the connector prefix | Added `fmt.Errorf("baton-newrelic: ...")` wrapping to all bare returns in both helpers | `pkg/newrelic/client.go:654-682,684-734` |

**CI fix (validate_metadata):** Added `Users` row to `docs/connector.mdx` capabilities table (Sync + Provision) to satisfy the docs-check that requires doc updates when capabilities change vs. main. Also added trailing newlines to `baton_capabilities.json` and `config_schema.json` (review nit).

**baton-test results after review-fixes (mock path — localhost:18080):**
```
baton-test run sync                      → PASS (4 resources, 2 entitlements, 1 grant)
baton-test run provisioning user --full  → PASS (create user-101, verify, delete, dup-delete, verify gone)
baton-test run action update_user        → PASS (name update invoked + completed)
```
