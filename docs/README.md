# Lerian CLI Documentation

Welcome to the Lerian CLI documentation! This directory contains comprehensive guides, references, and examples for using the Lerian command-line interface.

Lerian CLI is the command-line tool for signing in to the Lerian platform and deploying AWS infrastructure from the Lerian Terraform templates.

## Documentation Structure

### Getting Started
- [Installation Guide](getting-started/installation.md) *(coming soon)*
- [Quick Start](getting-started/quickstart.md) *(coming soon)*
- [Authentication](getting-started/authentication.md) *(coming soon)*
- [Configuration](getting-started/configuration.md) *(coming soon)*

### Command Reference
- [Authentication Commands](commands/auth.md) *(coming soon)*
- [Global Flags](commands/global-flags.md) *(coming soon)*

### User Guides

**General:**
- [Managing Profiles](guides/managing-profiles.md) *(coming soon)*
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
- [Testing Strategy](testing-strategy.md) - Comprehensive testing guide
- [Release Process](development/release-process.md) *(coming soon)*

### CI/CD
- [CI/CD Overview](ci-cd/README.md) - Workflow documentation
- [Workflow Summary](ci-cd/WORKFLOW_SUMMARY.md) - Complete workflow guide

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
- [GitHub Repository](https://github.com/LerianStudio/lerian-cli)
- [Issue Tracker](https://github.com/LerianStudio/lerian-cli/issues)
- [Discussions](https://github.com/LerianStudio/lerian-cli/discussions)

## Quick Start

For those who want to get started immediately:

```bash
# Install
go install github.com/LerianStudio/lerian-cli/cmd/lerian@latest

# Authenticate
lerian auth login \
  --api-url https://api.lerian.studio \
  --api-key YOUR_API_KEY \
  --tenant-id YOUR_TENANT_ID

# Check this machine
lerian infra check
```

See [Quick Start Guide](getting-started/quickstart.md) *(coming soon)* for detailed instructions.

## Command Overview

### Authentication (Global)

| Command | Description |
|---------|-------------|
| `lerian auth login` | Authenticate with API key and tenant ID |
| `lerian auth logout` | Clear authentication credentials |

### Infrastructure

See [`infra.md`](infra.md).

## Concepts

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
lerian --profile production config
```

## Examples

```bash
# Sign in with a named profile
lerian auth login --profile production --api-url https://api.lerian.studio --api-key $KEY --tenant-id $TENANT

# Verify the machine, then preview an environment without touching AWS
lerian infra check
lerian infra --env dev --target infra-base --dry-run
```

See [User Guides](guides/) for more examples.

## Troubleshooting

### Common Issues

**Authentication Failures**
- Verify API key and tenant ID in `~/.lerian/config.yaml`
- Ensure API endpoint is correct
- Check for expired credentials

**`lerian infra` fails before it starts**
- Run `lerian infra check`: it reports every missing dependency in one pass and makes no AWS call

**Command Not Found**
- Ensure CLI is in PATH: `which lerian`
- Reinstall: `go install github.com/LerianStudio/lerian-cli/cmd/lerian@latest`

See [Debugging Guide](guides/debugging.md) *(coming soon)* for detailed troubleshooting.

## Support

### Getting Help

- **Questions?** Open a [Discussion](https://github.com/LerianStudio/lerian-cli/discussions)
- **Bug?** Open an [Issue](https://github.com/LerianStudio/lerian-cli/issues)
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
- Documentation index (this file)
- README with quick start
- Contributing guidelines
- Code of conduct
- Security policy
- Changelog
- Testing strategy
- CI/CD workflows

### Phase 2 (Next)
- Getting started guides
- Command reference documentation
- User guides for common workflows
- Architecture documentation

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
