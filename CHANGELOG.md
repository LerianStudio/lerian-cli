## 1.0.0 (2025-11-25)


### Features

* add CI gate workflow to ensure all checks pass before release ([5c5d260](https://github.com/LerianStudio/lerian-cli/commit/5c5d26059ea02ab723629a863f667268bd980a09))
* add comprehensive testing, CI/CD workflows, and update Go to 1.25 ([d1fa5e5](https://github.com/LerianStudio/lerian-cli/commit/d1fa5e5b0d6cf2c5fc7bc93bd061b6d3898a784e))
* add semantic-release automation with CI/CD integration ([38c97cd](https://github.com/LerianStudio/lerian-cli/commit/38c97cdba75428512203b5685bb48101c474fff1))
* add version management system (Phase 7) ([22ed831](https://github.com/LerianStudio/lerian-cli/commit/22ed8316ec2225918aa491973ea0b73c52a3fb9b))
* extract lerian-cli source code from saas-poc (Phase 3) ([ff46dd2](https://github.com/LerianStudio/lerian-cli/commit/ff46dd2f240882886c50bfcead3ce51e6c9b44ba))
* update README badges for Go 1.24 and beta release version ([8592248](https://github.com/LerianStudio/lerian-cli/commit/8592248ce4e3cb1d87e9d0e32b78e06656cab1b1))
* upgrade to Go 1.25.4 ([2a61f7f](https://github.com/LerianStudio/lerian-cli/commit/2a61f7ffceb748b6e50443f25c4cb10d84d68d54))


### Bug Fixes

* add checks read permission to CI gate workflow ([febc035](https://github.com/LerianStudio/lerian-cli/commit/febc0356677268c4931ded52a9d91968b9945c9e))
* add release event trigger to go-release workflow ([48d74d2](https://github.com/LerianStudio/lerian-cli/commit/48d74d24025504e8b86502dff07b7717c793936c))
* comment out announce.github section incompatible with goreleaser v1.x ([6ffaa71](https://github.com/LerianStudio/lerian-cli/commit/6ffaa710e188db30f03d6d458b7d8456cf736eaf))
* comment out brews section since release is disabled ([2667d87](https://github.com/LerianStudio/lerian-cli/commit/2667d87e08af2a148e59e568cfe8d74816dd3e45))
* comment out docker sections requiring Dockerfile ([559ab1f](https://github.com/LerianStudio/lerian-cli/commit/559ab1f4c5586984626ca9d3aad99962f40e4a3a))
* comment out leftover lines from brews section causing YAML parsing error ([9b94285](https://github.com/LerianStudio/lerian-cli/commit/9b94285e083ced22aef35c70fbfc7a8e40139fdb))
* comment out sboms and signs sections requiring syft and cosign ([b31581d](https://github.com/LerianStudio/lerian-cli/commit/b31581d1649970dec6762245453ca4f9747dd1e3))
* disable goreleaser release creation, let semantic-release handle it ([adedefa](https://github.com/LerianStudio/lerian-cli/commit/adedefa6cad5a59aba5ab6637fd65823bd926694))
* downgrade .goreleaser.yml to version 1 format for compatibility with goreleaser v1.x ([f8fd883](https://github.com/LerianStudio/lerian-cli/commit/f8fd8836dbeaaf706bb942ab3f03ed95ce796e46))
* enable goreleaser release upload to attach binaries to github releases ([bb782a8](https://github.com/LerianStudio/lerian-cli/commit/bb782a876a8e198389ba4ec09fc4e649331d260f))
* map organization secrets to workflow parameters ([f237ea2](https://github.com/LerianStudio/lerian-cli/commit/f237ea2ebe48737808650a5f6ef792000670677a))
* pass manage_token secret to go-release workflow ([86dce02](https://github.com/LerianStudio/lerian-cli/commit/86dce02ecd2ddf1678a9ee2a05f93044333f5fef))
* remove double quotes in workflow files ([4dc06b7](https://github.com/LerianStudio/lerian-cli/commit/4dc06b74c194cf4b696ac4b353e0bb67776e92b4))
* remove header/footer from release section when disable is true ([66d6f3e](https://github.com/LerianStudio/lerian-cli/commit/66d6f3e6aa5fa754ab321644bfeddbb541a412c2))
* remove reserved github_token secret from go-release workflow ([9f8662e](https://github.com/LerianStudio/lerian-cli/commit/9f8662e5ad0fe55613b4fda6e040051a7e5f86cd))
* revert Go version from 1.25 to 1.23 ([17794ca](https://github.com/LerianStudio/lerian-cli/commit/17794ca9a48a6b2382606a31d8cb3ff2d226322c))
* revert to Go 1.23 and fix workflow configurations ([c8e5553](https://github.com/LerianStudio/lerian-cli/commit/c8e5553c94d7dce4ed949a4ccb0813ac788b668c))
* rewrite CI gate to use workflow_run trigger and github-script ([1bad7eb](https://github.com/LerianStudio/lerian-cli/commit/1bad7eb68a97dc2528365eaf5c964de9db9c8e55))
* simplify release header/footer templates to use v1-compatible variables ([be7dce4](https://github.com/LerianStudio/lerian-cli/commit/be7dce427993350e3a16afc281a1c44469ded6a6))
* simplify semantic-release trigger to use workflow_run ([07d4096](https://github.com/LerianStudio/lerian-cli/commit/07d40967b92dd6261f54819a5e0cf0f44d23dc11))
* use append mode for goreleaser release since semantic-release creates the release first ([ff33f5a](https://github.com/LerianStudio/lerian-cli/commit/ff33f5aa20900cd283e51586fea58ad94d860dac))
* use secrets inherit for semantic-release workflow ([8b90802](https://github.com/LerianStudio/lerian-cli/commit/8b90802215d60ef535a2c4ee938874c73b21fd47))
* use workflow_run trigger for semantic-release ([e9bc082](https://github.com/LerianStudio/lerian-cli/commit/e9bc082345b472b82e3c15fa95b69a201b325278))

## [1.0.0-beta.16](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.15...v1.0.0-beta.16) (2025-11-25)


### Bug Fixes

* enable goreleaser release upload to attach binaries to github releases ([bb782a8](https://github.com/LerianStudio/lerian-cli/commit/bb782a876a8e198389ba4ec09fc4e649331d260f))

## [1.0.0-beta.15](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.14...v1.0.0-beta.15) (2025-11-25)


### Bug Fixes

* comment out leftover lines from brews section causing YAML parsing error ([9b94285](https://github.com/LerianStudio/lerian-cli/commit/9b94285e083ced22aef35c70fbfc7a8e40139fdb))

## [1.0.0-beta.14](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.13...v1.0.0-beta.14) (2025-11-25)


### Features

* update README badges for Go 1.24 and beta release version ([8592248](https://github.com/LerianStudio/lerian-cli/commit/8592248ce4e3cb1d87e9d0e32b78e06656cab1b1))

## [1.0.0-beta.13](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.12...v1.0.0-beta.13) (2025-11-25)


### Bug Fixes

* remove header/footer from release section when disable is true ([66d6f3e](https://github.com/LerianStudio/lerian-cli/commit/66d6f3e6aa5fa754ab321644bfeddbb541a412c2))

## [1.0.0-beta.12](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.11...v1.0.0-beta.12) (2025-11-25)


### Bug Fixes

* comment out brews section since release is disabled ([2667d87](https://github.com/LerianStudio/lerian-cli/commit/2667d87e08af2a148e59e568cfe8d74816dd3e45))

## [1.0.0-beta.11](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.10...v1.0.0-beta.11) (2025-11-25)


### Bug Fixes

* disable goreleaser release creation, let semantic-release handle it ([adedefa](https://github.com/LerianStudio/lerian-cli/commit/adedefa6cad5a59aba5ab6637fd65823bd926694))

## [1.0.0-beta.10](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.9...v1.0.0-beta.10) (2025-11-25)


### Bug Fixes

* use append mode for goreleaser release since semantic-release creates the release first ([ff33f5a](https://github.com/LerianStudio/lerian-cli/commit/ff33f5aa20900cd283e51586fea58ad94d860dac))

## [1.0.0-beta.9](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.8...v1.0.0-beta.9) (2025-11-25)


### Bug Fixes

* simplify release header/footer templates to use v1-compatible variables ([be7dce4](https://github.com/LerianStudio/lerian-cli/commit/be7dce427993350e3a16afc281a1c44469ded6a6))

## [1.0.0-beta.8](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.7...v1.0.0-beta.8) (2025-11-25)


### Bug Fixes

* comment out docker sections requiring Dockerfile ([559ab1f](https://github.com/LerianStudio/lerian-cli/commit/559ab1f4c5586984626ca9d3aad99962f40e4a3a))

## [1.0.0-beta.7](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.6...v1.0.0-beta.7) (2025-11-25)


### Bug Fixes

* comment out sboms and signs sections requiring syft and cosign ([b31581d](https://github.com/LerianStudio/lerian-cli/commit/b31581d1649970dec6762245453ca4f9747dd1e3))

## [1.0.0-beta.6](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.5...v1.0.0-beta.6) (2025-11-25)


### Bug Fixes

* comment out announce.github section incompatible with goreleaser v1.x ([6ffaa71](https://github.com/LerianStudio/lerian-cli/commit/6ffaa710e188db30f03d6d458b7d8456cf736eaf))

## [1.0.0-beta.5](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.4...v1.0.0-beta.5) (2025-11-25)


### Bug Fixes

* downgrade .goreleaser.yml to version 1 format for compatibility with goreleaser v1.x ([f8fd883](https://github.com/LerianStudio/lerian-cli/commit/f8fd8836dbeaaf706bb942ab3f03ed95ce796e46))

## [1.0.0-beta.4](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.3...v1.0.0-beta.4) (2025-11-25)


### Bug Fixes

* pass manage_token secret to go-release workflow ([86dce02](https://github.com/LerianStudio/lerian-cli/commit/86dce02ecd2ddf1678a9ee2a05f93044333f5fef))

## [1.0.0-beta.3](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.2...v1.0.0-beta.3) (2025-11-25)


### Bug Fixes

* remove reserved github_token secret from go-release workflow ([9f8662e](https://github.com/LerianStudio/lerian-cli/commit/9f8662e5ad0fe55613b4fda6e040051a7e5f86cd))

## [1.0.0-beta.2](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0-beta.1...v1.0.0-beta.2) (2025-11-25)


### Bug Fixes

* add release event trigger to go-release workflow ([48d74d2](https://github.com/LerianStudio/lerian-cli/commit/48d74d24025504e8b86502dff07b7717c793936c))

## 1.0.0-beta.1 (2025-11-25)


### Features

* add comprehensive testing, CI/CD workflows, and update Go to 1.25 ([d1fa5e5](https://github.com/LerianStudio/lerian-cli/commit/d1fa5e5b0d6cf2c5fc7bc93bd061b6d3898a784e))
* add semantic-release automation with CI/CD integration ([38c97cd](https://github.com/LerianStudio/lerian-cli/commit/38c97cdba75428512203b5685bb48101c474fff1))
* add version management system (Phase 7) ([22ed831](https://github.com/LerianStudio/lerian-cli/commit/22ed8316ec2225918aa491973ea0b73c52a3fb9b))
* extract lerian-cli source code from saas-poc (Phase 3) ([ff46dd2](https://github.com/LerianStudio/lerian-cli/commit/ff46dd2f240882886c50bfcead3ce51e6c9b44ba))
* upgrade to Go 1.25.4 ([2a61f7f](https://github.com/LerianStudio/lerian-cli/commit/2a61f7ffceb748b6e50443f25c4cb10d84d68d54))


### Bug Fixes

* map organization secrets to workflow parameters ([f237ea2](https://github.com/LerianStudio/lerian-cli/commit/f237ea2ebe48737808650a5f6ef792000670677a))
* remove double quotes in workflow files ([4dc06b7](https://github.com/LerianStudio/lerian-cli/commit/4dc06b74c194cf4b696ac4b353e0bb67776e92b4))
* revert Go version from 1.25 to 1.23 ([17794ca](https://github.com/LerianStudio/lerian-cli/commit/17794ca9a48a6b2382606a31d8cb3ff2d226322c))
* revert to Go 1.23 and fix workflow configurations ([c8e5553](https://github.com/LerianStudio/lerian-cli/commit/c8e5553c94d7dce4ed949a4ccb0813ac788b668c))
* simplify semantic-release trigger to use workflow_run ([07d4096](https://github.com/LerianStudio/lerian-cli/commit/07d40967b92dd6261f54819a5e0cf0f44d23dc11))
* use secrets inherit for semantic-release workflow ([8b90802](https://github.com/LerianStudio/lerian-cli/commit/8b90802215d60ef535a2c4ee938874c73b21fd47))
* use workflow_run trigger for semantic-release ([e9bc082](https://github.com/LerianStudio/lerian-cli/commit/e9bc082345b472b82e3c15fa95b69a201b325278))

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
