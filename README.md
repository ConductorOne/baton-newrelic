![Baton Logo](./docs/images/baton-logo.png)

# `baton-newrelic` [![Go Reference](https://pkg.go.dev/badge/github.com/conductorone/baton-newrelic.svg)](https://pkg.go.dev/github.com/conductorone/baton-newrelic) ![main ci](https://github.com/conductorone/baton-newrelic/actions/workflows/main.yaml/badge.svg)

`baton-newrelic` is a connector for NewRelic built using the [Baton SDK](https://github.com/conductorone/baton-sdk). It communicates with the NewRelic GraphQL API, NerdGraph, to sync data about organizations, roles, groups and users. 

Check out [Baton](https://github.com/conductorone/baton) to learn more about the project in general.

# Prerequisites

To use the connector, you will need a NewRelic account with the admin permissions (like default Admin group) and a NewRelic API key. 

Authentication domain settings under Administration settings when configuring group permissions must be enabled for the API key to work.

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
  help               Help about any command

Flags:
      --apikey string          The API key used to connect to NewRelic GraphQL API. ($BATON_APIKEY)
      --client-id string       The client ID used to authenticate with ConductorOne ($BATON_CLIENT_ID)
      --client-secret string   The client secret used to authenticate with ConductorOne ($BATON_CLIENT_SECRET)
  -f, --file string            The path to the c1z file to sync with ($BATON_FILE) (default "sync.c1z")
  -h, --help                   help for baton-newrelic
      --log-format string      The output format for logs: json, console ($BATON_LOG_FORMAT) (default "json")
      --log-level string       The log level: debug, info, warn, error ($BATON_LOG_LEVEL) (default "info")
  -p, --provisioning           This must be set in order for provisioning actions to be enabled. ($BATON_PROVISIONING)
  -v, --version                version for baton-newrelic

Use "baton-newrelic [command] --help" for more information about a command.
```
