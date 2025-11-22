# Lerian CLI

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go)](https://go.dev/)
[![GitHub Release](https://img.shields.io/badge/release-v0.1.0-green.svg)](https://github.com/lerian-studio/lerian-cli/releases)

Official command-line interface for the Lerian platform. Manage Midaz ledger deployments, interact with your ledgers, and streamline your financial infrastructure operations.

## Features

- 🚀 **Ledger Management** - Create, list, describe, and delete Midaz ledger deployments
- 🔐 **Authentication** - Profile-based authentication with API key support
- ☁️ **Multi-Region Support** - Deploy to SaaS regions or private infrastructure
- 🎯 **Multiple Deployment Modes** - SaaS, private, and sandbox environments
- 📊 **Operations Tools** - Logs, port-forwarding, SQL execution, backups, and events
- 📝 **Multiple Output Formats** - Table, JSON, and YAML output options
- 🔧 **Kubernetes Integration** - Direct access to deployed resources

## Quick Start

### Installation

#### Using Go Install
```bash
go install github.com/lerian-studio/lerian-cli/cmd/lerian@latest
```

#### From Source
```bash
git clone https://github.com/lerian-studio/lerian-cli.git
cd lerian-cli
make install
```

#### Download Binary
Download the latest release for your platform from the [releases page](https://github.com/lerian-studio/lerian-cli/releases).

### First Steps

1. **Login to Lerian Platform**
   ```bash
   lerian auth login \
     --api-url https://api.lerian.studio \
     --api-key YOUR_API_KEY \
     --tenant-id YOUR_TENANT_ID
   ```

2. **Create Your First Ledger**
   ```bash
   lerian midaz ledger create \
     --name my-first-ledger \
     --region us-east-1 \
     --env dev
   ```

3. **List Your Ledgers**
   ```bash
   lerian midaz ledger list
   ```

4. **Get Ledger Details**
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

### Ledger Management

#### Create Ledger

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

#### List Ledgers

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

#### Describe Ledger

```bash
lerian midaz ledger describe <ledger-id>
```

#### Delete Ledger

```bash
lerian midaz ledger delete <ledger-id>
```

### Operations

#### View Logs

```bash
# Show logs
lerian midaz ledger logs <ledger-id>

# Follow logs in real-time
lerian midaz ledger logs <ledger-id> --follow

# Show last 100 lines
lerian midaz ledger logs <ledger-id> --tail 100
```

#### Port Forwarding

```bash
# Forward local port 8080 to ledger service port 8080
lerian midaz ledger port-forward <ledger-id> 8080:8080
```

#### Execute SQL

```bash
# Run SQL query
lerian midaz ledger exec <ledger-id> "SELECT COUNT(*) FROM transactions;"
```

#### Backup Database

```bash
# Create backup
lerian midaz ledger backup <ledger-id> --output ./backup.sql
```

#### View Kubernetes Events

```bash
# View events for troubleshooting
lerian midaz ledger events <ledger-id>
```

#### Check Available Versions

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

## Deployment Modes

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

## Ledger Sizes

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

## Development

### Prerequisites

- Go 1.21 or higher
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
