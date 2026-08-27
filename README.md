![Baton Logo](./docs/images/baton-logo.png)

# `baton-newrelic` [![Go Reference](https://pkg.go.dev/badge/github.com/conductorone/baton-newrelic.svg)](https://pkg.go.dev/github.com/conductorone/baton-newrelic) ![verify](https://github.com/conductorone/baton-newrelic/actions/workflows/verify.yaml/badge.svg)

`baton-newrelic` is a connector for NewRelic built using the [Baton SDK](https://github.com/conductorone/baton-sdk). It communicates with the NewRelic GraphQL API, NerdGraph, to sync data about organizations, roles, groups and users.

Check out [Baton](https://github.com/conductorone/baton) to learn more about the project in general.

# Prerequisites

To use the connector in full capacity, you will need a NewRelic account, at least one full platform user and the API key to act on behalf of that user.

You can create a new API key by logging into account and clicking on the profile tab in the left bottom corner. Then click on the API keys tab and create a new key.

# Getting Started

## brew

```
brew install conductorone/baton/baton conductorone/baton/baton-newrelic

BATON_APIKEY=apikey baton-newrelic
baton resources
```

## docker

```
docker run --rm -v $(pwd):/out -e BATON_APIKEY=apikey ghcr.io/conductorone/baton-newrelic:latest -f "/out/sync.c1z"
docker run --rm -v $(pwd):/out ghcr.io/conductorone/baton:latest -f "/out/sync.c1z" resources
```

## source

```
go install github.com/conductorone/baton/cmd/baton@main
go install github.com/conductorone/baton-newrelic/cmd/baton-newrelic@main

BATON_APIKEY=apikey baton-newrelic
baton resources
```

# Data Model

`baton-newrelic` will fetch information about the following NewRelic resources:

- Organizations
- Groups
- Roles
- Users

With `--provisioning` enabled, the connector can also:

- Create and delete NewRelic users
- Add users to groups and remove them
- Grant and revoke organization-, account- and group-scoped roles to groups

It registers one connector action:

- `update_user` — updates a user's full name, email address and/or tier. Takes `user_id` (required) plus at least one of `name`, `email` or `user_type`.

# Contributing, Support and Issues

We started Baton because we were tired of taking screenshots and manually building spreadsheets. We welcome contributions, and ideas, no matter how small -- our goal is to make identity and permissions sprawl less painful for everyone. If you have questions, problems, or ideas: Please open a Github Issue!

See [CONTRIBUTING.md](https://github.com/ConductorOne/baton/blob/main/CONTRIBUTING.md) for more details.

# `baton-newrelic` Command Line Usage

```
baton-newrelic

Usage:
  baton-newrelic [flags]
  baton-newrelic [command]

Available Commands:
  capabilities       Get connector capabilities
  completion         Generate the autocompletion script for the specified shell
  config             Get the connector config schema
  health-check       Check the health of a running connector
  help               Help about any command

Flags:
      --apikey string                                    required: The API key used to connect to NewRelic GraphQL API ($BATON_APIKEY)
      --auth-method string                               ($BATON_AUTH_METHOD)
      --client-id string                                 The client ID used to authenticate with ConductorOne ($BATON_CLIENT_ID)
      --client-secret string                             The client secret used to authenticate with ConductorOne ($BATON_CLIENT_SECRET)
      --external-resource-c1z string                     The path to the c1z file to sync external baton resources with ($BATON_EXTERNAL_RESOURCE_C1Z)
      --external-resource-entitlement-id-filter string   The entitlement that external users, groups must have access to sync external baton resources ($BATON_EXTERNAL_RESOURCE_ENTITLEMENT_ID_FILTER)
      --external-resource-traits strings                 Resource type traits (e.g. "user", "group", "app") to sync and match from the external resource c1z. When unset the matcher falls back to user and group; passing this flag replaces the full set rather than adding to it. ($BATON_EXTERNAL_RESOURCE_TRAITS)
  -f, --file string                                      The path to the c1z file to sync with ($BATON_FILE) (default "sync.c1z")
      --health-check                                     Enable the HTTP health check endpoint ($BATON_HEALTH_CHECK)
      --health-check-port int                            Port for the HTTP health check endpoint ($BATON_HEALTH_CHECK_PORT) (default 8081)
  -h, --help                                             help for baton-newrelic
      --http-timeout-seconds int                         HTTP client timeout in seconds (max 1800) ($BATON_HTTP_TIMEOUT_SECONDS) (default 300)
      --keep-previous-sync-c1z                           Keep the previously synced c1z on disk to enable ETag replay across service-mode syncs (requires a connector that supports ETag replay; costs one c1z of local disk) ($BATON_KEEP_PREVIOUS_SYNC_C1Z)
      --log-format string                                The output format for logs: json, console ($BATON_LOG_FORMAT) (default "json")
      --log-level string                                 The log level: debug, info, warn, error ($BATON_LOG_LEVEL) (default "info")
      --log-level-debug-expires-at string                The timestamp indicating when debug-level logging should expire ($BATON_LOG_LEVEL_DEBUG_EXPIRES_AT)
      --log-path strings                                 The file path to write logs to ($BATON_LOG_PATH)
      --otel-collector-endpoint string                   The endpoint of the OpenTelemetry collector to send observability data to (used for both tracing and logging if specific endpoints are not provided) ($BATON_OTEL_COLLECTOR_ENDPOINT)
      --parallel-sync                                    Deprecated: use --workers instead. ($BATON_PARALLEL_SYNC)
  -p, --provisioning                                     This must be set in order for provisioning actions to be enabled ($BATON_PROVISIONING)
      --skip-entitlements-and-grants                     This must be set to skip syncing of entitlements and grants ($BATON_SKIP_ENTITLEMENTS_AND_GRANTS)
      --skip-full-sync                                   This must be set to skip a full sync ($BATON_SKIP_FULL_SYNC)
      --storage-engine string                            The storage engine to use when opening the sync c1z file: sqlite or pebble. Defaults to pebble when unset. ($BATON_STORAGE_ENGINE)
      --sync-resource-types strings                      The resource type IDs to sync ($BATON_SYNC_RESOURCE_TYPES)
      --sync-resources strings                           The resource IDs to sync ($BATON_SYNC_RESOURCES)
      --task-concurrency int                             The number of Baton tasks to run concurrently in service mode. Tasks may include sync, grant, revoke, and more. Minimum value is 1, maximum value is 100. ($BATON_TASK_CONCURRENCY) (default 3)
      --ticketing                                        This must be set to enable ticketing support ($BATON_TICKETING)
  -v, --version                                          version for baton-newrelic
      --workers int                                      The number of sync workers to use. -1 for auto-detect, 0 for sequential, >0 for parallel ($BATON_WORKERS)

Use "baton-newrelic [command] --help" for more information about a command.
```
