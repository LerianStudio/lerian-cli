# Lerian CLI

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](https://go.dev/)
[![GitHub Release](https://img.shields.io/badge/release-v1.0.0--beta-green.svg)](https://github.com/lerian-studio/lerian-cli/releases)

Official command-line interface for the Lerian platform. Manage your infrastructure products and deployments.

## Supported Products

### Midaz (Available Now)

Midaz is a ledger system for managing assets, operations, and multi-tenancy environments.

**Features:**
- **Ledger Management** - Create, list, describe, and delete ledger deployments
- **Multi-Region Support** - Deploy to SaaS regions or private infrastructure
- **Multiple Deployment Modes** - SaaS, private, and sandbox environments
- **Operations Tools** - Logs, port-forwarding, SQL execution, backups, and events
- **Kubernetes Integration** - Direct access to deployed resources

### Infrastructure (Available Now)

Drive the Terraform roots of
[lerian-terraform-foundation](https://github.com/LerianStudio/lerian-terraform-foundation)
on AWS, from bootstrap to the per-product services.

**Features:**
- **Environment Bootstrap** - State bucket and lock table, then the VPC and the EKS cluster
- **Target Resolution** - Products and services discovered from the checkout; combine with commas
- **Account Guard** - Three checks before anything runs, with no flag to bypass them
- **Dry Run** - Resolve and print the whole execution plan without a single AWS call
- **Helm Values** - Read `helm_values` back out of the applied state

Ported from `lerian-infra-cli`, which this CLI replaces.

### Future Products

- **Flowker** - Coming soon
- **Reporter** - Coming soon
- **Tracer** - Coming soon
- **Fees** - Coming soon

## Core Features

- **Authentication** - Profile-based authentication with API key support
- **Multiple Output Formats** - Table, JSON, and YAML output options
- **Multi-Product Support** - Unified CLI for all Lerian products

## Installation

### Quick Install (Recommended)

The easiest way to install the Lerian CLI is using the install script:

```bash
curl -fsSL https://raw.githubusercontent.com/LerianStudio/lerian-cli/main/scripts/install.sh | sh
```

**Prerequisites:** [GitHub CLI (gh)](https://cli.github.com/) installed and authenticated with access to the repository.

#### Install Options

```bash
# Install latest version
curl -fsSL https://raw.githubusercontent.com/LerianStudio/lerian-cli/main/scripts/install.sh | sh

# Install specific version
curl -fsSL https://raw.githubusercontent.com/LerianStudio/lerian-cli/main/scripts/install.sh | sh -s -- --version v1.0.0

# Custom install directory
INSTALL_DIR=/usr/local/bin curl -fsSL https://raw.githubusercontent.com/LerianStudio/lerian-cli/main/scripts/install.sh | sh
```

### Other Installation Methods

<details>
<summary><strong>Manual Download</strong></summary>

Download the latest release for your platform from the [releases page](https://github.com/LerianStudio/lerian-cli/releases).

```bash
# Example for Linux amd64
gh release download --repo LerianStudio/lerian-cli --pattern "lerian_*_Linux_x86_64.tar.gz"
tar -xzf lerian_*_Linux_x86_64.tar.gz
sudo mv lerian /usr/local/bin/
```

</details>

<details>
<summary><strong>From Source</strong></summary>

```bash
git clone https://github.com/LerianStudio/lerian-cli.git
cd lerian-cli
make install
```

</details>

<details>
<summary><strong>Using Go Install</strong></summary>

```bash
go install github.com/LerianStudio/lerian-cli/cmd/lerian@latest
```

> Note: Requires Go 1.25+ and access to the private repository via GOPRIVATE.

</details>

<details>
<summary><strong>Linux Packages (.deb/.rpm)</strong></summary>

Download the appropriate package from the [releases page](https://github.com/LerianStudio/lerian-cli/releases):

```bash
# Debian/Ubuntu
gh release download --repo LerianStudio/lerian-cli --pattern "*.deb"
sudo dpkg -i lerian_*.deb

# RHEL/Fedora
gh release download --repo LerianStudio/lerian-cli --pattern "*.rpm"
sudo rpm -i lerian_*.rpm
```

</details>

### Verify Installation

```bash
lerian --version
```

### Shell Completions

Enable auto-completion for your shell:

```bash
# Bash
lerian completion bash > /etc/bash_completion.d/lerian
# Or for current user only:
lerian completion bash >> ~/.bashrc

# Zsh
lerian completion zsh > "${fpath[1]}/_lerian"
# Or add to ~/.zshrc:
echo 'source <(lerian completion zsh)' >> ~/.zshrc

# Fish
lerian completion fish > ~/.config/fish/completions/lerian.fish

# PowerShell
lerian completion powershell > lerian.ps1
```

## Quick Start

### First Steps

1. **Login to Lerian Platform**
   ```bash
   lerian auth login \
     --api-url https://api.lerian.studio \
     --api-key YOUR_API_KEY \
     --tenant-id YOUR_TENANT_ID
   ```

2. **Create Your First Midaz Ledger**
   ```bash
   lerian midaz ledger create \
     --name my-first-ledger \
     --region us-east-1 \
     --env dev
   ```

3. **List Your Midaz Ledgers**
   ```bash
   lerian midaz ledger list
   ```

4. **Get Midaz Ledger Details**
   ```bash
   lerian midaz ledger describe <ledger-id>
   ```

## Usage

### Global Flags

- `--config` - Config file path (default: `$HOME/.lerian/config.yaml`)
- `--profile, -p` - Profile to use (default: `default`)
- `--output, -o` - Output format: `table`, `json`, `yaml` (default: `table`)
- `--help, -h` - Show help for any command
- `--version, -v` - Show version information

### Authentication

```bash
# Login to platform
lerian auth login \
  --api-url https://api.lerian.studio \
  --api-key your-api-key \
  --tenant-id your-tenant-id

# Login with custom profile
lerian auth login \
  --profile production \
  --api-url https://api.lerian.studio \
  --api-key prod-key \
  --tenant-id prod-tenant

# Logout from current profile
lerian auth logout
```

### Infrastructure Commands

All infrastructure commands start with `lerian infra`. Unlike the rest of the CLI,
this group takes flags rather than subcommands — it kept the command line of the
`lerian-infra` binary it replaces, so anything written against that binary keeps
working with `lerian infra` in front of it.

```bash
# Verify this machine: dependencies, checkout, and what it would use
lerian infra check

# Write the configuration a fresh checkout needs
lerian infra init --env dev

# List the discoverable targets (no AWS call, no configuration read)
lerian infra --list

# Resolve and print the execution plan, touching nothing
lerian infra --env dev --target all --dry-run

# Stand up an environment, in order
lerian infra --env dev --target bootstrap        --action apply
lerian infra --env dev --target infra-base       --action apply
lerian infra --env dev --target shared-resources --action apply

# Read the helm values of a product back out
lerian infra --env dev --target midaz --action helm-values --format yaml
```

Run `lerian infra --help` for the full reference: every flag, the account guard,
the ordering rules, and the environment variables it reads.

### Midaz Product Commands

All Midaz commands start with `lerian midaz`.

#### Ledger Management

##### Create Ledger

**SaaS Deployment (Default):**
```bash
lerian midaz ledger create \
  --name my-ledger \
  --region us-east-1 \
  --env dev
```

**Private Deployment:**
```bash
lerian midaz ledger create \
  --name prod-ledger \
  --mode private \
  --region private-us-west-2 \
  --env prod \
  --size production \
  --agent-id <agent-uuid>
```

**Sandbox (7-day trial):**
```bash
lerian midaz ledger create \
  --name trial-ledger \
  --sandbox \
  --region us-east-1
```

**Available Options:**
- `--name` - Ledger name (required, 3-100 characters)
- `--region` - Deployment region (required)
- `--env` - Environment: `dev`, `staging`, `prod` (required unless `--sandbox`)
- `--mode` - Deployment mode: `saas`, `private` (default: `saas`)
- `--size` - Ledger size: `test`, `staging`, `production` (default: `test`)
- `--tps` - Transactions per second (10-10000)
- `--multi-az` - Enable multi-AZ deployment
- `--sandbox` - Create sandbox ledger with auto-expiration
- `--app-version` - Specific app version to deploy
- `--chart-version` - Specific Helm chart version
- `--agent-id` - Agent ID (required for private mode)

##### List Ledgers

```bash
# Table format (default)
lerian midaz ledger list

# JSON format
lerian midaz ledger list -o json

# YAML format
lerian midaz ledger list -o yaml

# Use specific profile
lerian midaz ledger list --profile production
```

##### Describe Ledger

```bash
lerian midaz ledger describe <ledger-id>
```

##### Delete Ledger

```bash
lerian midaz ledger delete <ledger-id>
```

#### Operations

##### View Logs

```bash
# Show logs
lerian midaz ledger logs <ledger-id>

# Follow logs in real-time
lerian midaz ledger logs <ledger-id> --follow

# Show last 100 lines
lerian midaz ledger logs <ledger-id> --tail 100
```

##### Port Forwarding

```bash
# Forward local port 8080 to ledger service port 8080
lerian midaz ledger port-forward <ledger-id> 8080:8080
```

##### Execute SQL

```bash
# Run SQL query
lerian midaz ledger exec <ledger-id> "SELECT COUNT(*) FROM accounts;"
```

##### Backup Database

```bash
# Create backup
lerian midaz ledger backup <ledger-id> --output ./backup.sql
```

##### View Kubernetes Events

```bash
# View events for troubleshooting
lerian midaz ledger events <ledger-id>
```

##### Check Available Versions

```bash
# List available app and chart versions
lerian midaz ledger versions
```

## Configuration

### Configuration File

Configuration is stored at `~/.lerian/config.yaml`:

```yaml
current-profile: default
profiles:
  default:
    api-url: https://api.lerian.studio
    api-key: your-api-key-here
    tenant-id: your-tenant-uuid-here
  production:
    api-url: https://api.lerian.studio
    api-key: prod-api-key
    tenant-id: prod-tenant-uuid
```

### Using Profiles

```bash
# Use production profile
lerian --profile production ledger list

# Set profile during login
lerian auth login --profile production --api-url ... --api-key ... --tenant-id ...
```

### Custom Config File

```bash
lerian --config /path/to/config.yaml ledger list
```

## Midaz Deployment Modes

Midaz ledgers support three deployment modes:

### SaaS Mode (Default)
Multi-tenant deployment on Lerian-managed infrastructure.
- Shared Kubernetes clusters
- Multiple availability zones
- Managed by Lerian team
- Quick provisioning

### Private Mode
Single-tenant deployment on your own infrastructure.
- Your Kubernetes cluster
- Full control over resources
- Data stays in your network
- Requires Lerian Agent

### Sandbox Mode
Temporary ledger for testing and trials.
- Auto-expires after 7 days
- Limited to test size
- Dev environment only
- Quick setup for evaluation

## Regions

### SaaS Regions
- `us-east-1` - US East (N. Virginia)
- `us-west-2` - US West (Oregon)
- `eu-west-1` - Europe (Ireland)
- `ap-southeast-1` - Asia Pacific (Singapore)
- `sa-east-1` - South America (São Paulo)

### Private Regions
- Use `private-*` prefix for agent-connected regions
- View available private regions: `lerian agent list` (future)
- Requires Lerian Agent deployment

## Midaz Ledger Sizes

| Size | TPS | Resources | Use Case |
|------|-----|-----------|----------|
| `test` | 10 | Minimal | Development and testing |
| `staging` | 100 | Medium | Pre-production environments |
| `production` | 1000 | Full | Production workloads |

## Troubleshooting

### Connection Issues

**Problem:** Cannot connect to Control Plane API

**Solution:**
```bash
# Verify API endpoint
curl https://api.lerian.studio/health

# Check configuration
cat ~/.lerian/config.yaml

# Re-authenticate
lerian auth login
```

### Authentication Errors

**Problem:** API returns 401 Unauthorized

**Solution:**
```bash
# Verify API key and tenant ID
cat ~/.lerian/config.yaml

# Login with correct credentials
lerian auth login --api-url ... --api-key ... --tenant-id ...
```

### Deployment Timeout

**Problem:** Ledger creation times out

**Solution:**
```bash
# Check deployment status
lerian midaz ledger describe <ledger-id>

# View events for more details
lerian midaz ledger events <ledger-id>
```

## Testing

### Test Coverage

Current test coverage: **~86%** (83.7% for config package, 90.9% for version package)

Target coverage: **80%+** for all packages

### Running Tests

```bash
# Run all tests with coverage
make test

# Run unit tests only (fast)
make test-unit

# Run tests with coverage report
make test-coverage

# Run tests with race detector
make test-race

# Run tests in verbose mode
make test-verbose

# Run benchmarks
make test-bench
```

### Test Organization

Tests follow Go best practices:
- **Unit tests**: Fast, isolated tests for individual functions
- **Table-driven tests**: Parameterized test cases for comprehensive coverage
- **Integration tests**: Tests with real file I/O and system interactions
- **Benchmark tests**: Performance testing

### Coverage by Package

| Package | Coverage | Target | Status |
|---------|----------|--------|--------|
| `internal/version` | 90.9% | 90%+ | Complete |
| `internal/config` | 83.7% | 80%+ | Complete |
| `internal/output` | 0% | 80%+ | In Progress |
| `internal/kubectl` | 0% | 70%+ | Planned |
| `internal/client` | 0% | 70%+ | Planned |
| `cmd/auth` | 0% | 60%+ | Planned |
| `cmd/midaz/ledger` | 0% | 60%+ | Planned |

### Test Examples

**Unit Test Example** (`internal/config/config_test.go`):
```go
func TestLoad_ValidConfigFile(t *testing.T) {
    // Setup: Create temporary config
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, ".lerian", "config.yaml")

    // Test: Load and verify
    config, err := Load()

    if err != nil {
        t.Fatalf("Load() error = %v", err)
    }
}
```

**Table-Driven Test Example**:
```go
func TestGetProfile(t *testing.T) {
    tests := []struct {
        name        string
        config      *Config
        profileName string
        wantErr     bool
    }{
        {"valid profile", validConfig, "test", false},
        {"profile not found", emptyConfig, "missing", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            profile, err := tt.config.GetProfile(tt.profileName)
            // assertions...
        })
    }
}
```

### Continuous Integration

Tests run automatically on:
- Pull requests to `develop`, `release-candidate`, and `main`
- Push to `develop`, `release-candidate`, and `main`
- Scheduled weekly security scans

See `.github/workflows/` for CI/CD configuration.

## Development

### Prerequisites

- Go 1.23 or higher
- kubectl (for Kubernetes operations)
- Make

### Building from Source

```bash
# Clone repository
git clone https://github.com/lerian-studio/lerian-cli.git
cd lerian-cli

# Install dependencies
make deps

# Build binary
make build

# Run tests
make test

# Install to $GOPATH/bin
make install
```

### Project Structure

```
lerian-cli/
├── cmd/
│   ├── lerian/          # Main entry point
│   ├── auth/            # Authentication commands
│   └── midaz/           # Midaz commands
│       └── ledger/      # Ledger management
├── internal/
│   ├── client/          # HTTP API client
│   ├── config/          # Configuration management
│   ├── kubectl/         # Kubernetes operations
│   └── output/          # Output formatting
├── docs/                # Documentation
├── examples/            # Example files
└── scripts/             # Build and automation scripts
```

## Documentation

- [Installation Guide](docs/getting-started/installation.md) *(coming soon)*
- [Command Reference](docs/commands/) *(coming soon)*
- [Architecture Overview](docs/architecture/overview.md) *(coming soon)*
- [Contributing Guide](CONTRIBUTING.md)
- [Security Policy](SECURITY.md)

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details on:
- Code of conduct
- Development setup
- Coding standards
- Pull request process
- Commit conventions

## Support

- **Documentation:** https://docs.lerian.studio *(coming soon)*
- **Issues:** [GitHub Issues](https://github.com/lerian-studio/lerian-cli/issues)
- **Community:** https://community.lerian.studio *(coming soon)*
- **Email:** support@lerian.studio

## License

Copyright © 2025 Lerian Studio. All rights reserved.

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.

## Acknowledgments

Built with:
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [YAML v3](https://github.com/go-yaml/yaml) - YAML support
- [UUID](https://github.com/google/uuid) - UUID generation

## Version History

See [CHANGELOG.md](CHANGELOG.md) for release history and changes.

---

**Current Version:** v0.1.0

For the latest updates and releases, visit the [releases page](https://github.com/lerian-studio/lerian-cli/releases).
