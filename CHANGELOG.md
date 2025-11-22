# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Comprehensive documentation (README, CONTRIBUTING, SECURITY, CODE_OF_CONDUCT)
- Standard Go project layout with `cmd/lerian/` structure
- Makefile build system improvements

### Changed
- Build directory moved from `bin/` to `build/bin/`
- Main entry point moved to `cmd/lerian/main.go`

## [0.1.0] - 2025-11-22

### Added

#### Authentication
- `lerian auth login` - Authenticate with Lerian Control Plane API
- `lerian auth logout` - Clear authentication credentials
- Profile-based configuration support with `~/.lerian/config.yaml`
- Multiple profile management (development, staging, production)
- API key and tenant ID authentication

#### Ledger Management
- `lerian midaz ledger create` - Create new Midaz ledger deployments
  - SaaS deployment mode (multi-tenant, managed infrastructure)
  - Private deployment mode (single-tenant, your infrastructure)
  - Sandbox mode (7-day trial ledgers)
  - Multi-region support (us-east-1, us-west-2, eu-west-1, ap-southeast-1, sa-east-1)
  - Configurable ledger sizes (test, staging, production)
  - Custom TPS configuration (10-10000)
  - Multi-AZ deployment option
  - Specific app and chart version selection
- `lerian midaz ledger list` - List all ledgers in tenant
- `lerian midaz ledger describe` - Get detailed information about a specific ledger
- `lerian midaz ledger delete` - Delete a ledger deployment
- `lerian midaz ledger versions` - Show available app and chart versions

#### Operations
- `lerian midaz ledger logs` - View ledger application logs
  - Real-time log streaming with `--follow`
  - Tail specific number of lines with `--tail`
- `lerian midaz ledger port-forward` - Forward local port to ledger service
- `lerian midaz ledger exec` - Execute SQL queries on ledger database
- `lerian midaz ledger backup` - Create database backup
- `lerian midaz ledger events` - View Kubernetes events for troubleshooting

#### Output Formats
- Table output (default, human-readable)
- JSON output (`--output json`)
- YAML output (`--output yaml`)

#### Configuration
- Profile-based configuration management
- Custom config file support with `--config` flag
- Environment-specific profiles (dev, staging, prod)
- Secure credential storage in `~/.lerian/config.yaml`

#### Integration
- HTTP client for Lerian Control Plane API
- Kubernetes integration via kubectl wrapper
- Deployment status polling with exponential backoff
- Request ID tracking for debugging

### Dependencies
- Go 1.21 or higher
- github.com/spf13/cobra v1.10.1 - CLI framework
- github.com/google/uuid v1.6.0 - UUID generation
- gopkg.in/yaml.v3 v3.0.1 - YAML configuration
- kubectl (runtime dependency for K8s operations)

### Documentation
- Comprehensive README with usage examples
- Installation guide (go install, source, binary)
- Quick start guide
- Command reference
- Configuration documentation
- Troubleshooting section
- Development setup guide

### Build System
- Makefile with build, install, test, clean targets
- Automated dependency installation
- Cross-platform binary building
- Standard Go project layout

## [0.1.0-extracted] - 2025-11-22

Initial extraction from saas-poc monorepo (commit 7c57531de254943).

### Extracted
- Core CLI implementation (25 Go files, 2,751 lines of code)
- Command structure (auth, midaz, ledger subcommands)
- Internal packages (client, config, kubectl, output)
- Build system (Makefile, go.mod, go.sum)
- Original implementation plan (moved to docs/HISTORICAL_IMPLEMENTATION_PLAN.md)

## Version History

### Version Numbering

This project follows [Semantic Versioning](https://semver.org/):
- **MAJOR** version for incompatible API changes
- **MINOR** version for backwards-compatible functionality additions
- **PATCH** version for backwards-compatible bug fixes

### Release Schedule

- **Major releases**: When significant breaking changes are needed
- **Minor releases**: Monthly (or when significant features are ready)
- **Patch releases**: As needed for critical bug fixes

### Support Policy

- **Current version** (0.1.x): Full support
- **Previous minor version**: Security patches only
- **Older versions**: No support (upgrade recommended)

## Migration Guides

### Migrating to 0.1.0 from saas-poc

If you were using `lerian-cli` from the saas-poc repository:

1. **Uninstall old CLI** (if installed globally):
   ```bash
   rm $(which lerian)
   ```

2. **Install new standalone CLI**:
   ```bash
   go install github.com/lerian-studio/lerian-cli/cmd/lerian@latest
   ```

3. **Configuration remains compatible** - Your `~/.lerian/config.yaml` works without changes

4. **All commands remain the same** - No breaking changes to command syntax

## Future Releases

### Planned for v0.2.0
- Unit tests for all commands
- Integration tests
- E2E tests
- GitHub Actions CI/CD pipeline
- Automated releases with GoReleaser
- Linux, macOS, Windows binaries
- Homebrew installation support
- Shell completion scripts (bash, zsh, fish, powershell)

### Planned for v0.3.0
- `lerian agent` commands for private deployment management
- Enhanced ledger monitoring and metrics
- Ledger scaling operations
- Automated backup scheduling
- Log aggregation and search

### Under Consideration
- Interactive mode for guided workflows
- Config validation and suggestions
- Ledger health checks and diagnostics
- Resource usage reporting
- Cost estimation
- Ledger cloning
- Blue-green deployment support

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for how to contribute to this project.

## Security

See [SECURITY.md](SECURITY.md) for security policies and vulnerability reporting.

---

**Legend:**
- `Added` - New features
- `Changed` - Changes in existing functionality
- `Deprecated` - Soon-to-be removed features
- `Removed` - Removed features
- `Fixed` - Bug fixes
- `Security` - Vulnerability fixes

[Unreleased]: https://github.com/lerian-studio/lerian-cli/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/lerian-studio/lerian-cli/releases/tag/v0.1.0
[0.1.0-extracted]: https://github.com/lerian-studio/lerian-cli/releases/tag/v0.1.0-extracted
