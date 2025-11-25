# Lerian CLI - CI/CD Pipeline Documentation

This document explains how the CI/CD pipelines work for the Lerian CLI project in simple, straightforward terms.

> **Note:** The workflows were reorganized on 2025-11-22 to follow Lerian Studio's naming conventions. See [REORGANIZATION.md](./REORGANIZATION.md) for details.

## Overview

The Lerian CLI uses **GitHub Actions** for automation. When you push code, create a pull request, or tag a release, automated workflows run to test, build, and release the software.

## Pipeline Architecture

```
┌─────────────────┐
│  Code Push/PR   │
└────────┬────────┘
         │
         ├─────────────────────────────────┐
         │                                 │
         ▼                                 ▼
┌──────────────────┐            ┌──────────────────┐
│  Pull Request    │            │   Main Branch    │
│    Pipeline      │            │     Pipeline     │
└────────┬─────────┘            └────────┬─────────┘
         │                               │
         ├─► Lint Code                   ├─► Build Binary
         ├─► Run Tests                   ├─► Run Tests
         ├─► Security Scan               └─► Security Scan
         ├─► Format Check                         │
         ├─► PR Validation                        │
         └─► Size Check                           │
                                                  │
                                                  ▼
                                         ┌─────────────────┐
                                         │   Tag v*.*.* │
                                         └────────┬────────┘
                                                  │
                                                  ▼
                                         ┌─────────────────┐
                                         │     Release     │
                                         │    Pipeline     │
                                         └────────┬────────┘
                                                  │
                                                  ├─► GoReleaser
                                                  ├─► Build Binaries
                                                  ├─► Create Release
                                                  ├─► Docker Images
                                                  └─► Homebrew Formula
```

## Workflows Explained

### 1. Go CI Workflow (`go-ci.yml`)

**When it runs:** On every push to `main` or `develop` branches, and on every pull request

**What it does:**
1. **Test Matrix** - Runs tests on Go 1.25 across multiple operating systems (Ubuntu, macOS, Windows)
2. **Build** - Compiles binaries for all supported platforms
3. **Lint** - Checks code quality with golangci-lint (40+ linters)
4. **Security** - Scans for security issues with Gosec
5. **Format** - Verifies code is properly formatted
6. **Coverage** - Uploads test coverage to Codecov

**Time:** ~5-10 minutes

**Result:** Green check = all quality gates passed ✅

### 2. Go Release Workflow (`go-release.yml`)

**When it runs:** When you push a git tag like `v0.1.0`, `v1.2.3`

**What it does:**
1. **Build Binaries** - Creates binaries for:
   - Linux (amd64, arm64, arm)
   - macOS (amd64, arm64)
   - Windows (amd64)
2. **Create Packages** - Generates:
   - Archives (tar.gz, zip)
   - Debian packages (.deb)
   - RPM packages (.rpm)
   - Alpine packages (.apk)
3. **Docker Images** - Builds and pushes multi-arch images to GitHub Container Registry
4. **Homebrew** - Updates Homebrew formula (if configured)
5. **GitHub Release** - Creates release with:
   - Changelog (automatically generated)
   - Download links for all platforms
   - Checksums for verification

**Time:** ~10-15 minutes

**Result:** New release published on GitHub 🎉

### 3. Go Security Workflow (`go-security.yml`)

**When it runs:**
- On every push and pull request
- Every Monday at midnight (scheduled)
- Can be triggered manually

**What it does:**
1. **Gosec** - Go-specific security scanner
2. **Govulncheck** - Official Go vulnerability database checker
3. **Nancy** - Dependency vulnerability scanner
4. **Trivy** - General security scanner
5. **TruffleHog** - Secret scanner (checks for leaked credentials)
6. **License Check** - Ensures dependencies have compatible licenses
7. **SBOM** - Generates Software Bill of Materials

**Time:** ~5-8 minutes

**Result:** Security report in GitHub Security tab 🔒

### 4. PR Validation Workflow (`pr-validation.yml`)

**When it runs:** On pull request events (open, update, ready for review)

**What it does:**
1. **Title Validation** - Ensures PR title follows Conventional Commits (e.g., `feat:`, `fix:`)
2. **Size Labeling** - Adds labels based on PR size (XS, S, M, L, XL)
3. **Description Check** - Validates PR has adequate description
4. **Auto-labeling** - Adds area labels based on changed files (auth, ledger, config, etc.)
5. **Changelog Check** - Reminds to update CHANGELOG.md
6. **Link Validation** - Checks for linked issues

**Time:** <1 minute

**Result:** Labels applied, validation feedback provided 🏷️

### 5. Code Analysis Workflow (`code-analysis.yml`)

**When it runs:**
- On push and pull requests
- Every Sunday at 3 AM (scheduled)
- Can be triggered manually

**What it does:**
1. **Advanced Code Analysis** - Scans for:
   - Security vulnerabilities
   - Code quality issues
   - Common programming errors
   - Performance problems
2. **SARIF Upload** - Uploads results to GitHub Security tab

**Time:** ~3-5 minutes

**Result:** Security findings in GitHub Security tab 🔍

## File Structure

```
.github/
├── workflows/
│   ├── go-ci.yml           # Main CI pipeline
│   ├── go-release.yml      # Release automation
│   ├── go-security.yml     # Security scanning
│   ├── pr-validation.yml   # PR validation
│   ├── code-analysis.yml   # Code analysis
│   └── archive/            # Old workflows (preserved)
├── ISSUE_TEMPLATE/
│   ├── bug_report.yml      # Bug report form
│   ├── feature_request.yml # Feature request form
│   └── config.yml          # Issue config
├── PULL_REQUEST_TEMPLATE.md
├── labeler.yml             # Auto-labeling rules
└── markdown-link-check-config.json
```

## Configuration Files

### `.golangci.yml`
**Purpose:** Configures code linting

**Key Settings:**
- 40+ enabled linters
- Line length: 120 characters
- Function complexity: 15
- Test file exemptions

### `.goreleaser.yml`
**Purpose:** Configures release automation

**Key Settings:**
- Build platforms
- Archive formats
- Package generation
- Docker image configuration
- Homebrew formula

### `Makefile`
**Purpose:** Local development tasks

**Commands:**
- `make build` - Build binary with version info
- `make test` - Run tests
- `make clean` - Clean build artifacts
- `make install` - Install to $GOPATH/bin

## How Versioning Works

The CLI uses **semantic versioning** (e.g., v1.2.3):

- **Major** (v1.0.0) - Breaking changes
- **Minor** (v0.1.0) - New features
- **Patch** (v0.0.1) - Bug fixes

**Version Detection:**
- Automatically detected from git tags using `git describe`
- Example: `v0.1.0-5-g5415fc6` means "5 commits after v0.1.0, commit 5415fc6"
- Injected at build time using Go linker flags

**Creating a Release:**
```bash
# 1. Update CHANGELOG.md with changes
# 2. Create and push tag
git tag v0.2.0
git push origin v0.2.0

# 3. GitHub Actions automatically:
#    - Runs all tests
#    - Builds binaries for all platforms
#    - Creates GitHub Release
#    - Publishes Docker images
#    - Updates Homebrew (if configured)
```

## Workflow Dependencies

```
Pull Request:
├─ pr-validation.yml (always runs first)
├─ go-ci.yml (runs in parallel)
├─ go-security.yml (runs in parallel)
└─ code-analysis.yml (runs in parallel)

Main Branch Push:
├─ go-ci.yml
├─ go-security.yml
└─ code-analysis.yml

Tag Push (v*.*.*):
└─ go-release.yml
    ├─ Runs tests first
    ├─ Builds binaries
    ├─ Creates release
    ├─ Publishes Docker images
    └─ Updates Homebrew
```

## Secrets Required

These secrets need to be configured in GitHub repository settings:

| Secret | Purpose | Required For |
|--------|---------|-------------|
| `CODECOV_TOKEN` | Upload test coverage | CI |
| `TAP_GITHUB_TOKEN` | Update Homebrew tap | Release |
| `GITHUB_TOKEN` | Automatic (GitHub provides) | All workflows |

## Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `VERSION` | Auto-detected from git | `v0.1.0` |
| `COMMIT` | Git commit hash | `5415fc6` |
| `DATE` | Build timestamp | `2025-11-22T14:23:14Z` |
| `BUILT_BY` | Builder identifier | `github-actions` |

## Status Badges

Add these to README.md to show pipeline status:

```markdown
[![CI](https://github.com/lerian-studio/lerian-cli/actions/workflows/go-ci.yml/badge.svg)](https://github.com/lerian-studio/lerian-cli/actions/workflows/go-ci.yml)
[![Security](https://github.com/lerian-studio/lerian-cli/actions/workflows/go-security.yml/badge.svg)](https://github.com/lerian-studio/lerian-cli/actions/workflows/go-security.yml)
[![Release](https://github.com/lerian-studio/lerian-cli/actions/workflows/go-release.yml/badge.svg)](https://github.com/lerian-studio/lerian-cli/actions/workflows/go-release.yml)
```

## Troubleshooting

### CI Fails on Tests
**Problem:** Tests fail in GitHub Actions but pass locally

**Solutions:**
- Check Go version matches (use 1.25+)
- Run tests with race detector: `go test -race ./...`
- Check for flaky tests
- Review test output in Actions log

### Release Fails to Build
**Problem:** GoReleaser fails during release

**Solutions:**
- Verify `.goreleaser.yml` syntax
- Check all platforms build locally: `GOOS=linux GOARCH=amd64 go build ./cmd/lerian`
- Review GoReleaser documentation
- Check build logs for specific errors

### Security Scan Failures
**Problem:** Security workflow reports vulnerabilities

**Solutions:**
- Run `go list -m all` to see dependencies
- Update dependencies: `go get -u ./...`
- Check specific vulnerability in GitHub Security tab
- Add exceptions in `.golangci.yml` if false positive

### Docker Image Build Fails
**Problem:** Docker multi-arch build fails

**Solutions:**
- Verify Dockerfile syntax
- Test build locally: `docker build -t lerian-cli .`
- Check platform support: `docker buildx ls`
- Review error in release workflow logs

## Best Practices

### Before Pushing Code
```bash
# 1. Format code
gofmt -w .

# 2. Run linter
golangci-lint run

# 3. Run tests
go test ./...

# 4. Run security checks
go vet ./...

# 5. Build binary
make build
```

### Before Creating Release
```bash
# 1. Update CHANGELOG.md
# 2. Update version in documentation
# 3. Test build
make clean && make build
# 4. Test binary
./build/bin/lerian version
# 5. Create tag
git tag v0.2.0
git push origin v0.2.0
```

### Code Quality Checklist
- [ ] All tests passing locally
- [ ] Code formatted (`gofmt -w .`)
- [ ] No linter warnings
- [ ] No `go vet` warnings
- [ ] CHANGELOG.md updated
- [ ] Documentation updated if needed
- [ ] Commit message follows conventions

## Continuous Improvement

The CI/CD pipeline is continuously improved based on:
- Team feedback
- Performance metrics
- New security requirements
- Tool updates

To suggest improvements, open an issue or pull request.

## Additional Resources

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [GoReleaser Documentation](https://goreleaser.com/)
- [Conventional Commits](https://www.conventionalcommits.org/)
- [Semantic Versioning](https://semver.org/)
- [GolangCI-Lint](https://golangci-lint.run/)

## Questions?

If you have questions about the CI/CD pipeline:
1. Check this documentation first
2. Review workflow files in `.github/workflows/`
3. Open a discussion in GitHub Discussions
4. Contact the maintainers

---

**Last Updated:** 2025-11-22
**Version:** 1.0.0
