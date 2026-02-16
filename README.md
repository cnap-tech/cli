# CNAP CLI

Command-line interface for managing CNAP workspaces, clusters, and deployments.

## Install

```bash
# Homebrew (macOS / Linux)
brew install cnap-tech/tap/cnap

# Go
go install github.com/cnap-tech/cli/cmd/cnap@latest

# GitHub Releases — download binary for your platform
# https://github.com/cnap-tech/cli/releases
```

## Quick Start

```bash
# Authenticate with a Personal Access Token
cnap auth login --token cnap_pat_...

# List and select a workspace
cnap workspaces list
cnap workspaces switch <workspace-id>

# Manage clusters
cnap clusters list
cnap clusters get <cluster-id>
```

## Configuration

Config is stored at `~/.cnap/config.yaml`. Environment variables take priority:

| Env Var | Description |
|---------|-------------|
| `CNAP_API_TOKEN` | API token (overrides config) |
| `CNAP_API_URL` | API base URL (overrides config) |

## Global Flags

| Flag | Description |
|------|-------------|
| `-o, --output` | Output format: `table`, `json`, `quiet` |
| `--api-url` | API base URL override |

## Commands

| Command | Description |
|---------|-------------|
| `cnap auth login --token <token>` | Authenticate |
| `cnap auth logout` | Remove credentials |
| `cnap auth status` | Show auth status |
| `cnap workspaces list` | List workspaces |
| `cnap workspaces switch <id>` | Set active workspace |
| `cnap clusters list` | List clusters |
| `cnap clusters get <id>` | Get cluster details |
| `cnap clusters update <id>` | Update cluster |
| `cnap clusters delete <id> --force` | Delete cluster |

## Development

Prerequisites: [mise](https://mise.jdx.dev) for tool management.

```bash
# Install tools (Go, golangci-lint, goreleaser, task)
mise install

# Regenerate API client from OpenAPI spec
task generate

# Run all checks (vet + lint + test)
task check

# Individual commands
task build          # Build binary
task lint           # Run golangci-lint
task vet            # Run go vet
task fmt            # Format code
task test           # Run tests
task release:snapshot  # Build snapshot release locally
task clean          # Remove build artifacts
```

The API client is auto-generated from the OpenAPI spec at `internal/api/openapi.json` using [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen).
