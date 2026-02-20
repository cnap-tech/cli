# CNAP CLI

Command-line interface for managing CNAP workspaces, clusters, and deployments.

## Install

```bash
# Homebrew (macOS / Linux) — includes shell completions
brew install cnap-tech/tap/cnap

# Go
go install github.com/cnap-tech/cli/cmd/cnap@latest

# GitHub Releases — download binary for your platform
# https://github.com/cnap-tech/cli/releases
```

## Quick Start

```bash
# Authenticate via browser (stores session token)
cnap auth login

# Or authenticate with a Personal Access Token (for CI/CD)
cnap auth login --token cnap_pat_...

# List and select a workspace
cnap workspaces list
cnap workspaces switch <workspace-id>

# Manage clusters
cnap clusters list
cnap clusters get <cluster-id>

# Deploy a product
cnap installs create --product <product-id> --region <region-id>

# Stream logs
cnap installs logs <install-id> --follow
```

## Shell Completions

Homebrew installs completions automatically. For manual installs:

### Zsh

```bash
# Current session
source <(cnap completion zsh)

# Permanent (macOS with Homebrew)
cnap completion zsh > $(brew --prefix)/share/zsh/site-functions/_cnap

# Permanent (Linux)
cnap completion zsh > "${fpath[1]}/_cnap"
```

If completions don't work, ensure `compinit` is loaded in your `~/.zshrc`:

```bash
autoload -U compinit; compinit
```

### Bash

Requires the `bash-completion` package (`brew install bash-completion@2` on macOS).

```bash
# Current session
source <(cnap completion bash)

# Permanent (macOS with Homebrew)
cnap completion bash > $(brew --prefix)/etc/bash_completion.d/cnap

# Permanent (Linux)
cnap completion bash > /etc/bash_completion.d/cnap
```

### Fish

```bash
# Current session
cnap completion fish | source

# Permanent
cnap completion fish > ~/.config/fish/completions/cnap.fish
```

## Configuration

Config is stored at `~/.cnap/config.yaml`. Environment variables take priority:

| Env Var | Description |
|---------|-------------|
| `CNAP_API_TOKEN` | API token — PAT or session token (overrides config) |
| `CNAP_API_URL` | API base URL (overrides config) |
| `CNAP_AUTH_URL` | Auth base URL (overrides config) |
| `CNAP_DEBUG` | Enable debug logging (set to any value) |
| `CNAP_NO_UPDATE_NOTIFIER` | Disable update notifications (set to any value) |

## Global Flags

| Flag | Description |
|------|-------------|
| `-o, --output` | Output format: `table`, `json`, `quiet` |
| `--api-url` | API base URL override |
| `--debug` | Enable debug logging (HTTP traces to stderr) |

## Commands

All resource commands support singular and plural forms (e.g. `cnap cluster` or `cnap clusters`),
short aliases (e.g. `cl`, `inst`, `tpl`), and `ls` as an alias for `list`.

When run interactively without an ID argument, commands show a picker to select a resource.
Delete commands prompt for confirmation unless `--yes`/`-y` is passed.

| Command | Description |
|---------|-------------|
| **Auth** | |
| `cnap auth login` | Authenticate via browser (stores session token) |
| `cnap auth login --token <token>` | Authenticate with a PAT |
| `cnap auth logout` | Remove credentials (revokes session) |
| `cnap auth status` | Show auth status and token type |
| **Workspaces** | |
| `cnap workspaces list` | List workspaces |
| `cnap workspaces switch [id]` | Set active workspace |
| **Clusters** | |
| `cnap clusters list` | List clusters |
| `cnap clusters get [id]` | Get cluster details |
| `cnap clusters update [id]` | Update cluster |
| `cnap clusters delete [id]` | Delete cluster (confirms interactively) |
| `cnap clusters kubeconfig [id]` | Download admin kubeconfig |
| **Templates** | |
| `cnap templates list` | List templates |
| `cnap templates get [id]` | Get template with helm sources |
| `cnap templates delete [id]` | Delete template (confirms interactively) |
| **Products** | |
| `cnap products list` | List products |
| `cnap products get [id]` | Get product details |
| `cnap products delete [id]` | Delete product (confirms interactively) |
| **Installs** | |
| `cnap installs list` | List installs |
| `cnap installs get [id]` | Get install details |
| `cnap installs create --product <id> --region <id>` | Create product install |
| `cnap installs update-values [id] --source <id> -f values.yaml` | Update template values |
| `cnap installs update-overrides [id] --source <id> -f values.yaml` | Update install overrides |
| `cnap installs delete [id]` | Delete install (confirms interactively) |
| `cnap installs pods [id]` | List pods |
| `cnap installs logs [id] [--pod X] [--follow] [--tail N]` | Stream logs |
| `cnap installs exec [id] [--pod X] [--container X]` | Open interactive shell in pod |
| **Regions** | |
| `cnap regions list` | List regions |
| `cnap regions create --name <name>` | Create region |
| **Registry** | |
| `cnap registry list` | List registry credentials |
| `cnap registry delete [id]` | Delete registry credential (confirms interactively) |
| **Shell Completions** | |
| `cnap completion bash` | Generate bash completions |
| `cnap completion zsh` | Generate zsh completions |
| `cnap completion fish` | Generate fish completions |

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
