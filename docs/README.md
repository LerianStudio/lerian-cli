# Lerian CLI Documentation

Welcome to the Lerian CLI documentation! This directory contains comprehensive guides, references, and examples for using the Lerian command-line interface.

## Documentation Structure

### Getting Started
- [Installation Guide](getting-started/installation.md) *(coming soon)*
- [Quick Start](getting-started/quickstart.md) *(coming soon)*
- [Authentication](getting-started/authentication.md) *(coming soon)*
- [Configuration](getting-started/configuration.md) *(coming soon)*

### Command Reference
- [Authentication Commands](commands/auth.md) *(coming soon)*
- [Ledger Management](commands/ledger.md) *(coming soon)*
- [Operations Commands](commands/operations.md) *(coming soon)*
- [Global Flags](commands/global-flags.md) *(coming soon)*

### User Guides
- [Creating Ledgers](guides/creating-ledgers.md) *(coming soon)*
- [Managing Profiles](guides/managing-profiles.md) *(coming soon)*
- [Multi-Region Deployments](guides/multi-region.md) *(coming soon)*
- [Private Deployments](guides/private-deployments.md) *(coming soon)*
- [Debugging and Troubleshooting](guides/debugging.md) *(coming soon)*
- [CI/CD Integration](guides/cicd-integration.md) *(coming soon)*

### Architecture
- [Overview](architecture/overview.md) *(coming soon)*
- [Control Plane Integration](architecture/control-plane.md) *(coming soon)*
- [Kubernetes Operations](architecture/kubernetes.md) *(coming soon)*
- [Configuration Management](architecture/config.md) *(coming soon)*

### Development
- [Development Setup](development/setup.md) *(coming soon)*
- [Contributing Guidelines](../CONTRIBUTING.md)
- [Code Structure](development/code-structure.md) *(coming soon)*
- [Adding Commands](development/adding-commands.md) *(coming soon)*
- [Testing Guide](development/testing.md) *(coming soon)*
- [Release Process](development/release-process.md) *(coming soon)*

### API Reference
- [HTTP Client](api/http-client.md) *(coming soon)*
- [Configuration API](api/config-api.md) *(coming soon)*
- [Kubectl Wrapper](api/kubectl-wrapper.md) *(coming soon)*
- [Output Formatters](api/output-formatters.md) *(coming soon)*

## Quick Links

### Essential Reading
- [README](../README.md) - Main project documentation
- [CONTRIBUTING](../CONTRIBUTING.md) - How to contribute
- [CODE_OF_CONDUCT](../CODE_OF_CONDUCT.md) - Community guidelines
- [SECURITY](../SECURITY.md) - Security policies
- [CHANGELOG](../CHANGELOG.md) - Version history

### External Resources
- [Lerian Platform Documentation](https://docs.lerian.studio) *(coming soon)*
- [Midaz Documentation](https://docs.midaz.io) *(coming soon)*
- [GitHub Repository](https://github.com/lerian-studio/lerian-cli)
- [Issue Tracker](https://github.com/lerian-studio/lerian-cli/issues)
- [Discussions](https://github.com/lerian-studio/lerian-cli/discussions)

## Quick Start

For those who want to get started immediately:

```bash
# Install
go install github.com/lerian-studio/lerian-cli/cmd/lerian@latest

# Authenticate
lerian auth login \
  --api-url https://api.lerian.studio \
  --api-key YOUR_API_KEY \
  --tenant-id YOUR_TENANT_ID

# Create a ledger
lerian midaz ledger create \
  --name my-first-ledger \
  --region us-east-1 \
  --env dev

# List ledgers
lerian midaz ledger list
```

See [Quick Start Guide](getting-started/quickstart.md) *(coming soon)* for detailed instructions.

## Command Overview

### Authentication

| Command | Description |
|---------|-------------|
| `lerian auth login` | Authenticate with API key and tenant ID |
| `lerian auth logout` | Clear authentication credentials |

### Ledger Management

| Command | Description |
|---------|-------------|
| `lerian midaz ledger create` | Create a new ledger deployment |
| `lerian midaz ledger list` | List all ledgers |
| `lerian midaz ledger describe <id>` | Get ledger details |
| `lerian midaz ledger delete <id>` | Delete a ledger |
| `lerian midaz ledger versions` | List available versions |

### Operations

| Command | Description |
|---------|-------------|
| `lerian midaz ledger logs <id>` | View ledger logs |
| `lerian midaz ledger port-forward <id>` | Forward port to ledger |
| `lerian midaz ledger exec <id> <sql>` | Execute SQL query |
| `lerian midaz ledger backup <id>` | Create database backup |
| `lerian midaz ledger events <id>` | View Kubernetes events |

See [Command Reference](commands/) for complete documentation.

## Concepts

### Deployment Modes

**SaaS Mode** (default)
- Multi-tenant deployments on Lerian-managed infrastructure
- Quick provisioning in available regions
- Managed by Lerian team
- Ideal for: Development, staging, small-scale production

**Private Mode**
- Single-tenant deployments on your infrastructure
- Full control over resources and data location
- Requires Lerian Agent installation
- Ideal for: Enterprise, compliance-sensitive, large-scale production

**Sandbox Mode**
- Temporary ledgers for testing and evaluation
- Auto-expires after 7 days
- Limited to test size and dev environment
- Ideal for: Trials, demos, quick experiments

### Regions

**SaaS Regions:**
- `us-east-1` - US East (N. Virginia)
- `us-west-2` - US West (Oregon)
- `eu-west-1` - Europe (Ireland)
- `ap-southeast-1` - Asia Pacific (Singapore)
- `sa-east-1` - South America (São Paulo)

**Private Regions:**
- Custom regions connected via Lerian Agent
- Use `private-*` prefix (e.g., `private-us-west-2`)

### Ledger Sizes

| Size | TPS | Resources | Use Case |
|------|-----|-----------|----------|
| `test` | 10 | Minimal | Development and testing |
| `staging` | 100 | Medium | Pre-production environments |
| `production` | 1000 | Full | Production workloads |

### Profiles

Profiles allow managing multiple environments:

```yaml
# ~/.lerian/config.yaml
current-profile: development

profiles:
  development:
    api-url: https://api.lerian.studio
    api-key: dev_key
    tenant-id: dev_tenant

  production:
    api-url: https://api.lerian.studio
    api-key: prod_key
    tenant-id: prod_tenant
```

Use profiles with `--profile` flag:
```bash
lerian --profile production ledger list
```

## Examples

### Creating a Development Ledger

```bash
lerian midaz ledger create \
  --name dev-ledger \
  --region us-east-1 \
  --env dev \
  --size test
```

### Creating a Production Ledger with Multi-AZ

```bash
lerian midaz ledger create \
  --name prod-ledger \
  --region us-east-1 \
  --env prod \
  --size production \
  --tps 5000 \
  --multi-az
```

### Creating a Private Ledger

```bash
lerian midaz ledger create \
  --name enterprise-ledger \
  --mode private \
  --region private-us-west-2 \
  --env prod \
  --size production \
  --agent-id <agent-uuid>
```

### Viewing Logs in Real-Time

```bash
lerian midaz ledger logs <ledger-id> --follow --tail 100
```

### Port Forwarding for Development

```bash
# Forward local port 8080 to ledger port 8080
lerian midaz ledger port-forward <ledger-id> 8080:8080

# Access ledger locally
curl http://localhost:8080/health
```

See [User Guides](guides/) for more examples.

## Troubleshooting

### Common Issues

**Authentication Failures**
- Verify API key and tenant ID in `~/.lerian/config.yaml`
- Ensure API endpoint is correct
- Check for expired credentials

**Deployment Timeouts**
- Check Control Plane status
- Verify network connectivity
- Review Kubernetes events: `lerian midaz ledger events <id>`

**Command Not Found**
- Ensure CLI is in PATH: `which lerian`
- Reinstall: `go install github.com/lerian-studio/lerian-cli/cmd/lerian@latest`

See [Debugging Guide](guides/debugging.md) *(coming soon)* for detailed troubleshooting.

## Support

### Getting Help

- **Questions?** Open a [Discussion](https://github.com/lerian-studio/lerian-cli/discussions)
- **Bug?** Open an [Issue](https://github.com/lerian-studio/lerian-cli/issues)
- **Email:** support@lerian.studio
- **Documentation:** https://docs.lerian.studio *(coming soon)*

### Security Issues

Report security vulnerabilities to **security@lerian.studio**.

See [SECURITY.md](../SECURITY.md) for our security policy.

## Contributing

We welcome contributions! Please see:
- [Contributing Guidelines](../CONTRIBUTING.md)
- [Code of Conduct](../CODE_OF_CONDUCT.md)
- [Development Setup](development/setup.md) *(coming soon)*

## Historical Documentation

- [Historical Implementation Plan](HISTORICAL_IMPLEMENTATION_PLAN.md) - Original implementation from saas-poc extraction

## Documentation Roadmap

The following documentation is planned:

### Phase 1 (Current)
- ✅ Documentation index (this file)
- ✅ README with quick start
- ✅ Contributing guidelines
- ✅ Code of conduct
- ✅ Security policy
- ✅ Changelog

### Phase 2 (Next)
- [ ] Getting started guides
- [ ] Command reference documentation
- [ ] User guides for common workflows
- [ ] Architecture documentation

### Phase 3 (Future)
- [ ] API reference documentation
- [ ] Development guides
- [ ] Video tutorials
- [ ] Interactive examples

## License

Copyright © 2025 Lerian Studio. All rights reserved.

Licensed under the Apache License, Version 2.0. See [LICENSE](../LICENSE) for details.

---

**Last Updated:** 2025-11-22
**Version:** 0.1.0
