# Lerian CLI

A unified command-line interface for managing your Lerian platform services including Midaz, Finflow, and Finbase.

## Overview

Lerian CLI provides a comprehensive toolset for:
- Creating and managing Midaz ledger deployments
- Interacting with ledger resources
- Managing authentication and configuration
- Accessing logs and debugging tools

## Installation

### Prerequisites

- Go 1.25.4 or higher
- Access to Lerian Control Plane API
- Valid API credentials

### Building from Source

```bash
# Clone the repository
git clone https://github.com/lerian-studio/lerian-cli.git
cd lerian-cli

# Build the CLI
go build -o bin/lerian .

# (Optional) Install globally
sudo mv bin/lerian /usr/local/bin/
```

### Using Make

```bash
# Build the CLI
make build

# Install to /usr/local/bin
make install
```

## Configuration

### Authentication

Before using the CLI, you need to authenticate:

```bash
lerian auth login
```

This will prompt for:
- Control Plane URL (default: http://localhost:8080)
- API Key
- Tenant ID

### Configuration File

Configuration is stored at `~/.lerian/config.yaml`:

```yaml
control_plane_url: http://localhost:8080
api_key: your-api-key-here
tenant_id: your-tenant-uuid-here
```

You can specify a custom config file:

```bash
lerian --config /path/to/config.yaml [command]
```

### Profiles

Use profiles to manage multiple environments:

```bash
lerian --profile production ledger list
```

## Commands

### Global Flags

- `--config string`: Config file (default: $HOME/.lerian/config.yaml)
- `--output string, -o`: Output format - table, json, yaml (default: table)
- `--profile string, -p`: Profile to use (default: default)
- `--help, -h`: Help for any command
- `--version, -v`: Show version information

## Creating Ledgers

Create a new Midaz ledger deployment using the `ledger create` command.

### Basic Usage

```bash
lerian midaz ledger create \
  --name <ledger-name> \
  --region <region> \
  --env <environment>
```

### Deployment Modes

#### SaaS Mode (Default)
Multi-tenant deployment on shared infrastructure.

```bash
lerian midaz ledger create \
  --name my-ledger \
  --region us-east-1 \
  --env dev
```

#### Private Mode
Dedicated single-tenant deployment.

```bash
lerian midaz ledger create \
  --name my-private-ledger \
  --mode private \
  --region private-us-west-2 \
  --env prod \
  --size production
```

#### Sandbox Mode
Quick trial ledger with auto-expiration (7 days).

```bash
lerian midaz ledger create \
  --name trial-ledger \
  --sandbox \
  --region us-east-1
```

### Available Flags

**Required:**
- `--name`: Ledger name (3-100 characters)
- `--region`: Deployment region (e.g., us-east-1, eu-west-1, private-*)
- `--env`: Environment type (dev, staging, prod)

**Optional:**
- `--mode`: Deployment mode - saas or private (default: saas)
- `--size`: Ledger size - test, staging, production (default: test)
- `--tps`: Transactions per second, 10-10000 (default: based on size)
- `--multi-az`: Enable multi-AZ deployment (default: false)
- `--sandbox`: Create sandbox ledger with auto-expiration
- `--app-version`: Specific app version to deploy
- `--chart-version`: Specific Helm chart version to use

### Size Options

| Size | TPS | Description | Use Case |
|------|-----|-------------|----------|
| test | 10 | Minimal resources | Development and testing |
| staging | 100 | Medium resources | Pre-production environments |
| production | 1000 | Full resources | Production workloads |

### Regions

**SaaS Regions:**
- us-east-1 (US East - N. Virginia)
- us-west-2 (US West - Oregon)
- eu-west-1 (Europe - Ireland)
- ap-southeast-1 (Asia Pacific - Singapore)
- sa-east-1 (South America - Sao Paulo)

**Private Regions:**
- Use `private-*` prefix for connected agent regions
- View available private regions with `lerian agent list`

### Examples

**Development ledger:**
```bash
lerian midaz ledger create \
  --name dev-ledger \
  --region us-east-1 \
  --env dev \
  --size test
```

**Production ledger with high throughput:**
```bash
lerian midaz ledger create \
  --name prod-ledger \
  --region us-west-2 \
  --env prod \
  --size production \
  --tps 5000 \
  --multi-az
```

**Quick sandbox for testing:**
```bash
lerian midaz ledger create \
  --name quick-test \
  --sandbox \
  --region us-east-1
```

### Provisioning

The command polls deployment status every 5 seconds until:
- Status becomes "available" (success)
- Status becomes "failed" (error)
- Timeout after 5 minutes (error)

Progress updates are displayed during polling.

## Managing Ledgers

### List All Ledgers

```bash
lerian midaz ledger list
```

Output formats:
```bash
lerian midaz ledger list -o json
lerian midaz ledger list -o yaml
```

### Describe a Ledger

Get detailed information about a specific ledger:

```bash
lerian midaz ledger describe <ledger-id>
```

### View Ledger Logs

Stream logs from the ledger service:

```bash
lerian midaz ledger logs <ledger-id>

# Follow logs in real-time
lerian midaz ledger logs <ledger-id> --follow

# Show last N lines
lerian midaz ledger logs <ledger-id> --tail 100
```

### Port Forwarding

Forward a local port to access the ledger service directly:

```bash
lerian midaz ledger port-forward <ledger-id> 8080:8080
```

### Execute SQL Commands

Run SQL commands against the ledger database:

```bash
lerian midaz ledger exec <ledger-id> "SELECT COUNT(*) FROM transactions;"
```

### Backup Ledger Database

Create a backup of the ledger PostgreSQL database:

```bash
lerian midaz ledger backup <ledger-id> --output ./backup.sql
```

### View Kubernetes Events

View Kubernetes events for troubleshooting:

```bash
lerian midaz ledger events <ledger-id>
```

## Troubleshooting

### Connection Refused

**Problem:** Cannot connect to Control Plane API

**Solutions:**
```bash
# Verify Control Plane is running
curl http://localhost:8080/health

# Check configuration
cat ~/.lerian/config.yaml

# Verify correct URL in config
lerian auth login
```

### API Authentication Error

**Problem:** API returns 401 Unauthorized

**Solutions:**
```bash
# Re-authenticate
lerian auth login

# Verify API key is valid
cat ~/.lerian/config.yaml

# Check tenant ID exists
# Contact administrator to verify credentials
```

### Deployment Timeout

**Problem:** Ledger creation times out after 5 minutes

**Solutions:**
```bash
# Check Control Plane logs
# (from control-plane directory)
tail -f logs/control-plane.log

# Verify agent is connected (for private regions)
lerian agent list

# Check deployment status manually
lerian midaz ledger describe <ledger-id>
```

### Validation Error

**Problem:** Command returns validation error

**Solutions:**
```bash
# Check flag values match requirements
lerian midaz ledger create --help

# Verify required flags are provided
lerian midaz ledger create \
  --name my-ledger \
  --region us-east-1 \
  --env dev

# Ensure name length is 3-100 characters
# Ensure env is one of: dev, staging, prod
# Ensure tps is between 10-10000 (if specified)
```

### Sandbox Expiration

**Problem:** Sandbox ledger stopped working after 7 days

**Explanation:** Sandbox ledgers automatically expire after 7 days to prevent resource waste.

**Solutions:**
```bash
# Create a new sandbox ledger
lerian midaz ledger create \
  --name new-sandbox \
  --sandbox \
  --region us-east-1

# Or create a permanent ledger
lerian midaz ledger create \
  --name permanent-ledger \
  --region us-east-1 \
  --env dev
```

## Development

### Building

```bash
# Build for current platform
go build -o bin/lerian .

# Build for specific platform
GOOS=linux GOARCH=amd64 go build -o bin/lerian-linux-amd64 .
GOOS=darwin GOARCH=arm64 go build -o bin/lerian-darwin-arm64 .
GOOS=windows GOARCH=amd64 go build -o bin/lerian-windows-amd64.exe .
```

### Testing

```bash
# Run unit tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific test
go test -v ./internal/client -run TestCreateDeployment
```

### Dependencies

```bash
# Update dependencies
go mod tidy

# Add new dependency
go get github.com/example/package

# View dependency graph
go mod graph
```

## Architecture

### Components

- **CLI Layer**: Cobra-based command structure
- **Client Layer**: HTTP client for Control Plane API
- **Config Layer**: Configuration and credential management
- **Display Layer**: Output formatting (table, JSON, YAML)

### API Integration

The CLI communicates with the Lerian Control Plane API:
- **Endpoint**: Configurable (default: http://localhost:8080)
- **Authentication**: API key-based
- **Format**: JSON request/response
- **Tenant Isolation**: All operations are scoped to tenant ID

### Data Flow

```
User Command
    |
    v
Cobra Command Handler
    |
    v
Input Validation
    |
    v
Config Loading
    |
    v
API Client (HTTP)
    |
    v
Control Plane API
    |
    v
Response Processing
    |
    v
Output Formatting
    |
    v
Display to User
```

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Make your changes
4. Run tests: `go test ./...`
5. Commit with conventional commits: `git commit -m "feat(cli): add new feature"`
6. Push to your fork: `git push origin feat/my-feature`
7. Create a Pull Request

### Commit Convention

Use conventional commits format:
- `feat(scope): description` - New feature
- `fix(scope): description` - Bug fix
- `docs(scope): description` - Documentation
- `refactor(scope): description` - Code refactoring
- `test(scope): description` - Tests
- `chore(scope): description` - Maintenance

## License

Copyright (c) 2025 Lerian Studio. All rights reserved.

## Support

- Documentation: https://docs.lerian.studio
- Issues: https://github.com/lerian-studio/lerian-cli/issues
- Community: https://community.lerian.studio

## Version History

### v0.1.0 (2025-11-17)
- Initial release
- Ledger create command with SaaS, private, and sandbox modes
- Authentication and configuration management
- Basic ledger management commands (list, describe, logs, etc.)
