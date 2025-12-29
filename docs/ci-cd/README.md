# Lerian CLI - CI/CD Pipeline Documentation

This document explains how the CI/CD pipelines work for the Lerian CLI project with visual flowcharts and comprehensive workflow descriptions.

> **Note:** The workflows were reorganized on 2025-11-22 and simplified on 2025-11-26 to separate PR validation from release automation.

## Table of Contents

- [Pipeline Architecture](#pipeline-architecture)
- [GitFlow Branching Model](#gitflow-branching-model)
- [Workflow Descriptions](#workflow-descriptions)
- [Semantic Versioning](#semantic-versioning)
- [Configuration Files](#configuration-files)
- [Secrets and Variables](#secrets-and-variables)
- [Troubleshooting](#troubleshooting)
- [Best Practices](#best-practices)

## Pipeline Architecture

The Lerian CLI uses **GitHub Actions** with a streamlined CI/CD pipeline that separates PR validation from release automation.

### Pipeline Overview

The pipeline has two distinct flows:

1. **Pull Request Flow**: Quality checks run on every PR
2. **Release Flow**: Automated releases on push to protected branches

### Pull Request Flow

```
┌─────────────────────────────────────┐
│  PR: opened/edited/synchronize/     │
│           reopened                  │
└───────────────┬─────────────────────┘
                │
        ┌───────┼───────┬────────┬────────────┐
        │       │       │        │            │
        ▼       ▼       ▼        ▼            ▼
    ┌───────┐ ┌────┐ ┌────┐ ┌────────┐ ┌──────────┐
    │Go CI  │ │Unit│ │Cov │ │  Go    │ │    PR    │
    │       │ │Test│ │Chk │ │Security│ │Validation│
    └───────┘ └────┘ └────┘ └────────┘ └──────────┘
         All run on source branch
         Must pass before merge
```

### Release Flow

```
┌─────────────────────────────────────┐
│  Push to develop/RC/main            │
└───────────────┬─────────────────────┘
                │
                ▼
        ┌───────────────┐
        │   Semantic    │
        │    Release    │
        └───────┬───────┘
                │
                │ (Create Tag v*.*.*)
                ▼
        ┌───────────────┐
        │  Go Release   │
        └───────┬───────┘
                │
        ┌───────┼────────┬──────────┐
        │       │        │          │
        ▼       ▼        ▼          ▼
    ┌──────┐ ┌────┐ ┌────────┐ ┌──────┐
    │Build │ │Pack│ │Checksum│ │Upload│
    └──────┘ └────┘ └────────┘ └──────┘
                │
                ▼
        ┌───────────────┐
        │GitHub Release │
        └───────────────┘
```

### Detailed Pipeline Flow

```
Developer        GitHub       Semantic Release     Go Release
    │                │                 │                 │
    │  Create PR     │                 │                 │
    │─────────────>  │                 │                 │
    │                │                 │                 │
    │          ┌─────┴─────┐           │                 │
    │          │   Run CI  │           │                 │
    │          │  Checks   │           │                 │
    │          │ (4 flows) │           │                 │
    │          └─────┬─────┘           │                 │
    │                │                 │                 │
    │  PR passes     │                 │                 │
    │  & merged      │                 │                 │
    │─────────────>  │                 │                 │
    │                │                 │                 │
    │          Push to branch          │                 │
    │                │─────────────────>                 │
    │                │                 │                 │
    │                │       Analyze commits             │
    │                │       Generate version            │
    │                │       Update CHANGELOG            │
    │                │                 │                 │
    │      Create Tag & Release        │                 │
    │                │ <───────────────┘                 │
    │                │                                   │
    │                │  Tag created event                │
    │                │───────────────────────────────────>
    │                │                                   │
    │                │           Build multi-platform    │
    │                │           binaries                │
    │                │           Create packages         │
    │                │           Generate checksums      │
    │                │                                   │
    │       Upload release assets                        │
    │                │ <─────────────────────────────────┘
    │                │                                   │
```

## GitFlow Branching Model

The project follows a three-branch GitFlow strategy with automated semantic versioning.

### Branch Strategy Diagram

```
main:                ●──────────────────────────────────────●
                     │                                      │
                     │                                  [merge RC]
                     │                                  v1.0.0
                     │                                      │
                     │                                      │
release-candidate:   │           ●──────────●───────────────┘
                     │           │          │
                     │       [merge dev] [fix RC]
                     │       v1.0.0-rc.1 v1.0.0-rc.2
                     │           │
                     │           │
develop:             ●───────────●───────────●───────────●───────────●
                    Initial    feat A      fix B      feat C     feat D
                               v1.0.0-     v1.0.0-    v1.0.0-    v1.0.1-
                               beta.1      beta.2     beta.3     beta.1

Timeline: ────────────────────────────────────────────────>
```

### Branch Lifecycle

```
┌─────────────────┐    Merge when     ┌─────────────────┐    Merge when    ┌─────────────────┐
│    develop      │      stable       │release-candidate│      tested      │      main       │
│                 │──────────────────>│                 │─────────────────>│                 │
│ Beta Releases   │                   │   RC Releases   │                  │Stable Releases  │
└─────────────────┘                   └─────────────────┘                  └─────────────────┘
         │                                     │                                     │
         │ v1.0.0-beta.X                       │ v1.0.0-rc.X                         │ v1.0.0
         └─────────────┐                       └─────────────┐                       └─────────────┐
                       │                                     │                                     │
                       │                                     │                      Hotfix         │
                       │                                     │                      if needed      │
                       │                                     │                       ┌─────────────┘
                       └─> (self-tag)                        └─> (self-tag)          └────> (self-tag)
```

### Branch Details

| Branch | Purpose | Release Type | Version Pattern | Trigger |
|--------|---------|--------------|-----------------|---------|
| **develop** | Active development | Pre-release (beta) | `v1.0.0-beta.X` | Every push |
| **release-candidate** | Release testing | Pre-release (RC) | `v1.0.0-rc.X` | Merge from develop |
| **main** | Production-ready | Stable release | `v1.0.0` | Merge from RC |

### Merge Strategy

1. **Feature → develop**: Create PR, pass all checks, merge
2. **develop → release-candidate**: Fast-forward merge when ready for RC
3. **release-candidate → main**: Fast-forward merge after RC validation
4. **Hotfix → main**: Direct commit to main for critical fixes

## Workflow Descriptions

### 1. Go CI (`go-ci.yml`)

**Triggers:**
- Pull requests: `opened`, `edited`, `synchronize`, `reopened`
- Target branches: `develop`, `release-candidate`, `main`

**What it does:**
- **Cross-platform testing**: Tests on Go 1.24 across Ubuntu and macOS
- **Build verification**: Compiles binaries for all platforms
- **Code linting**: Runs 40+ linters with golangci-lint
- **Format check**: Verifies code formatting
- **Documentation check**: Validates documentation consistency
- **Module verification**: Ensures go.mod and go.sum are tidy

**Duration:** ~3-5 minutes

**Configuration:** Uses shared workflow from `LerianStudio/github-actions-shared-workflows`

### 2. Unit Tests (`unit-tests.yml`)

**Triggers:**
- Pull requests: `opened`, `edited`, `synchronize`, `reopened`
- Target branches: `develop`, `release-candidate`, `main`

**What it does:**
- **Unit test execution**: Runs all unit tests
- **Race detection**: Tests with race detector enabled
- **Fast execution**: Focused on unit tests only
- **Parallel execution**: Tests run independently

**Duration:** ~2-3 minutes

**Configuration:** Uses shared workflow from `LerianStudio/github-actions-shared-workflows`

### 3. Coverage Check (`coverage.yml`)

**Triggers:**
- Pull requests: `opened`, `edited`, `synchronize`, `reopened`
- Target branches: `develop`, `release-candidate`, `main`

**What it does:**
- **Coverage calculation**: Measures test coverage
- **Threshold enforcement**: Fails if coverage < 25%
- **PR comments**: Posts coverage report on PRs
- **Trend tracking**: Monitors coverage over time

**Duration:** ~2-3 minutes

**Configuration:** Minimum coverage threshold: 25%, Target: 80%+

### 4. Go Security (`go-security.yml`)

**Triggers:**
- Pull requests: `opened`, `edited`, `synchronize`, `reopened`
- Target branches: `develop`, `release-candidate`, `main`
- Schedule: Every Monday at 00:00 UTC
- Manual dispatch

**What it does:**
- **Gosec**: Go-specific security scanner
- **Govulncheck**: Official Go vulnerability checker
- **Nancy**: Dependency vulnerability scanner
- **Trivy**: Container and code security scanner
- **Secret scanning**: Detects leaked credentials
- **License check**: Validates dependency licenses
- **SBOM generation**: Creates Software Bill of Materials

**Duration:** ~4-6 minutes

**Configuration:** Fails on security issues, uploads SARIF to GitHub Security

**Note:** Dependency Review is disabled (requires GitHub Advanced Security)

### 5. PR Validation (`pr-validation.yml`)

**Triggers:**
- Pull request events: `opened`, `synchronize`, `reopened`, `ready_for_review`

**What it does:**
- **Title validation**: Ensures conventional commit format
- **Description check**: Validates PR has adequate description (min 50 chars)
- **Auto-labeling**: Adds labels based on:
  - PR size (XS/S/M/L/XL)
  - Changed areas (auth, ledger, config, etc.)
- **Changelog reminder**: Checks if CHANGELOG.md needs update
- **Link validation**: Verifies linked issues

**Duration:** <1 minute

**Valid PR title types:** `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`, `ci`, `build`, `revert`

### 6. Semantic Release (`semantic-release.yml`)

**Triggers:**
- Push to: `develop`, `release-candidate`, `main`
- Manual dispatch

**What it does:**
- **Commit analysis**: Analyzes commit messages since last release
- **Version calculation**: Determines next version based on conventional commits
  - `feat:` → Minor version bump
  - `fix:` → Patch version bump
  - `BREAKING CHANGE:` → Major version bump
- **CHANGELOG generation**: Auto-generates CHANGELOG.md
- **Git tag creation**: Creates version tag (e.g., `v1.0.0-beta.1`)
- **GitHub release**: Creates GitHub release with notes
- **Branch-specific versions**:
  - `develop` → beta releases
  - `release-candidate` → RC releases
  - `main` → stable releases

**Duration:** ~1-2 minutes

**Note:** Runs independently on push events, does not wait for PR validation workflows

### 7. Go Release (`go-release.yml`)

**Triggers:**
- Release published (created by semantic-release)
- Tag push matching `v*.*.*`

**What it does:**
- **Pre-flight checks**: Runs tests before building
- **Multi-platform builds**: Creates binaries for:
  - Linux: amd64, arm64, armv7
  - macOS: amd64 (Intel), arm64 (Apple Silicon)
  - Windows: amd64
- **Package creation**:
  - Archives: `.tar.gz` (Unix), `.zip` (Windows)
  - Linux packages: `.deb`, `.rpm`, `.apk`
- **Asset upload**: Uploads all artifacts to GitHub Release
- **Checksum generation**: Creates SHA256 checksums

**Duration:** ~8-12 minutes

**Output:** GitHub release with 16+ downloadable assets

## Semantic Versioning

The project follows [Semantic Versioning 2.0.0](https://semver.org/).

### Version Format

```
v<MAJOR>.<MINOR>.<PATCH>[-<PRERELEASE>][+<BUILDMETADATA>]

Examples:
- v1.0.0          (stable release)
- v1.0.0-beta.1   (beta pre-release)
- v1.0.0-rc.2     (release candidate)
```

### Version Calculation

Based on [Conventional Commits](https://www.conventionalcommits.org/):

| Commit Type | Version Impact | Example |
|-------------|----------------|---------|
| `feat:` | Minor (+0.1.0) | `feat: add user authentication` |
| `fix:` | Patch (+0.0.1) | `fix: resolve login timeout` |
| `BREAKING CHANGE:` | Major (+1.0.0) | `feat!: redesign API` |
| `chore:`, `docs:`, etc. | No version change | `chore: update dependencies` |

### Branch-Specific Versions

```
                   ┌──────────────┐
                   │Commit Types  │
                   └──────┬───────┘
                          │
                          ▼
                    ┌─────────┐
                    │ Branch? │
                    └────┬────┘
                         │
         ┌───────────────┼───────────────┐
         │               │               │
      develop   release-candidate      main
         │               │               │
         ▼               ▼               ▼
  ┌──────────────┐ ┌──────────────┐ ┌────────────┐
  │v1.0.0-beta.X │ │v1.0.0-rc.X   │ │  v1.0.0    │
  └──────┬───────┘ └──────┬───────┘ └─────┬──────┘
         │                │                │
    ┌────┴────┐      ┌────┴────┐      ┌───┴────┐
    │         │      │         │      │        │
    │  feat:  │      │  feat:  │      │ feat:  │
    │ +minor  │      │ +minor  │      │ +minor │
    │         │      │         │      │        │
    │  fix:   │      │  fix:   │      │  fix:  │
    │ +patch  │      │ +patch  │      │ +patch │
    └─────────┘      └─────────┘      └────────┘
    (beta.X++)         (rc.X++)       (patch++)
```

## Configuration Files

### Workflow Configuration

```
.github/
├── workflows/
│   ├── go-ci.yml              # Main CI pipeline
│   ├── unit-tests.yml         # Unit test execution
│   ├── coverage.yml           # Coverage checking
│   ├── go-security.yml        # Security scanning
│   ├── pr-validation.yml      # PR validation
│   ├── semantic-release.yml   # Automated versioning
│   └── go-release.yml         # Release automation
├── ISSUE_TEMPLATE/
├── PULL_REQUEST_TEMPLATE.md
└── labeler.yml
```

### Build Configuration

| File | Purpose | Key Settings |
|------|---------|-------------|
| `.goreleaser.yml` | Release automation | Multi-platform builds, packaging |
| `.golangci.yml` | Code linting | 40+ linters, complexity limits |
| `.releaserc.json` | Semantic release | Commit analysis, version rules |
| `Makefile` | Local development | Build, test, install commands |

## Secrets and Variables

### Required Secrets

| Secret | Purpose | Used By | Setup Required |
|--------|---------|---------|----------------|
| `LERIAN_STUDIO_MIDAZ_PUSH_BOT_APP_ID` | GitHub App authentication | Semantic Release | ✓ |
| `LERIAN_STUDIO_MIDAZ_PUSH_BOT_PRIVATE_KEY` | GitHub App private key | Semantic Release | ✓ |
| `LERIAN_CI_CD_USER_GPG_KEY` | Commit signing | Semantic Release | ✓ |
| `LERIAN_CI_CD_USER_GPG_KEY_PASSWORD` | GPG key password | Semantic Release | ✓ |
| `LERIAN_CI_CD_USER_NAME` | Committer name | Semantic Release | ✓ |
| `LERIAN_CI_CD_USER_EMAIL` | Committer email | Semantic Release | ✓ |
| `MANAGE_TOKEN` | GitHub PAT | Coverage, Release | ✓ |
| `GITHUB_TOKEN` | Automatic token | All workflows | Auto |

### Environment Variables

Automatically set during builds:

| Variable | Description | Example |
|----------|-------------|---------|
| `VERSION` | Git-based version | `v1.0.0-beta.1` |
| `COMMIT` | Git commit SHA | `1bad7eb` |
| `DATE` | Build timestamp | `2025-11-25T20:28:45Z` |
| `BUILT_BY` | Builder | `goreleaser` |

## Troubleshooting

### Common Issues

#### 1. Semantic Release Doesn't Trigger

**Symptom:** Push to branch but no release created

**Cause:** No releasable commits since last release

**Solution:**
- Ensure commits follow conventional format (`feat:`, `fix:`, etc.)
- Verify semantic-release workflow triggered: `gh run list --workflow=semantic-release.yml`
- Check semantic-release logs for analysis result
- Confirm conventional commits exist since last tag

#### 2. CI Workflows Don't Run on PR

**Symptom:** PR created but CI workflows don't start

**Cause:** PR trigger configuration or branch mismatch

**Solution:**
```bash
# Check PR details
gh pr view <pr-number>

# Verify PR targets correct branch (develop/release-candidate/main)
gh pr view <pr-number> --json baseRefName

# Check workflow run history
gh run list --limit 10
```

#### 3. Security Scan Fails

**Symptom:** Go Security workflow reports vulnerabilities

**Cause:** Dependencies or code have security issues

**Solution:**
```bash
# Run security scans locally
make security-check

# Update vulnerable dependencies
go get -u ./...
go mod tidy

# Check specific vulnerability
govulncheck ./...
```

#### 4. Coverage Below Threshold

**Symptom:** Coverage Check fails with "Coverage below 25%"

**Solution:**
```bash
# Check coverage locally
go test -cover ./...

# Get detailed coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Add tests for uncovered code
```

#### 5. Release Assets Missing

**Symptom:** GitHub release created but no binaries attached

**Cause:** Go Release workflow didn't complete

**Solution:**
```bash
# Check Go Release workflow status
gh run list --workflow=go-release.yml --limit 5

# View failure logs
gh run view <run-id> --log

# Verify .goreleaser.yml configuration
goreleaser check
```

### Debug Commands

```bash
# List recent workflow runs
gh run list --limit 20

# View specific workflow run
gh run view <run-id>

# Check workflow logs
gh run view <run-id> --log

# List releases
gh release list

# View release details
gh release view <tag>

# Check git tags
git tag -l "v*" --sort=-version:refname
```

## Best Practices

### Before Pushing Code

```bash
# 1. Run tests locally
make test

# 2. Check code coverage
go test -cover ./...

# 3. Run linter
golangci-lint run

# 4. Format code
gofmt -w .

# 5. Build locally
make build

# 6. Verify binary works
./build/bin/lerian version
```

### Commit Message Format

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

**Examples:**
```
feat(auth): add OAuth2 authentication
fix(ledger): resolve balance calculation error
docs: update CI/CD documentation
chore: bump dependencies to latest versions
```

### Pull Request Checklist

- [ ] PR title follows conventional commit format
- [ ] Description explains what and why (min 50 chars)
- [ ] All CI workflows passing (Go CI, Unit Tests, Coverage, Security)
- [ ] Test coverage maintained or improved
- [ ] Documentation updated if needed
- [ ] CHANGELOG.md updated (if applicable)
- [ ] No security vulnerabilities introduced
- [ ] Code reviewed and approved

### Release Checklist

**For Beta Release (develop):**
- [ ] Feature branch merged to develop via PR
- [ ] All PR CI checks passed
- [ ] Conventional commits used
- [ ] Push to develop triggers semantic-release
- [ ] Semantic-release creates beta tag
- [ ] Go Release builds and uploads assets

**For RC Release (release-candidate):**
- [ ] Develop branch stable and tested
- [ ] Create PR: develop → release-candidate
- [ ] All CI checks pass on PR
- [ ] Merge PR to release-candidate
- [ ] Push triggers semantic-release
- [ ] Semantic-release creates RC tag

**For Stable Release (main):**
- [ ] RC thoroughly tested
- [ ] Create PR: release-candidate → main
- [ ] All CI checks pass on PR
- [ ] Merge PR to main
- [ ] Push triggers semantic-release
- [ ] Semantic-release creates stable tag
- [ ] Release notes reviewed
- [ ] Announcement prepared

## Workflow Dependencies Graph

```
PR Flow:                        Push Flow:
┌─────────────┐                ┌─────────────┐
│  Create PR  │                │  Push Code  │
└──────┬──────┘                └──────┬──────┘
       │                              │
       ├──────┬──────┬──────┐         │
       │      │      │      │         │
       ▼      ▼      ▼      ▼         ▼
   ┌────┐ ┌────┐ ┌───┐ ┌────┐   ┌──────────┐
   │Go  │ │Unit│ │Cov│ │ Go │   │Semantic  │
   │CI  │ │Test│ │Chk│ │Sec │   │Release   │
   └─┬──┘ └─┬──┘ └─┬─┘ └─┬──┘   └────┬─────┘
     │      │      │     │           │
     └──────┴──────┴─────┘           │
            │                        │
            ▼                        ▼
      ┌──────────┐            ┌──────────┐
      │All Pass? │            │Create Tag│
      └─────┬────┘            └────┬─────┘
            │                      │
            ▼                      ▼
      ┌──────────┐            ┌──────────┐
      │Can Merge │            │Go Release│
      └──────────┘            └────┬─────┘
                                   │
                    ┌──────────────┼──────────────┐
                    │              │              │
                    ▼              ▼              ▼
               ┌────────┐    ┌─────────┐    ┌────────┐
               │ Build  │    │ Package │    │Checksum│
               │Binaries│    │         │    │        │
               └────┬───┘    └────┬────┘    └───┬────┘
                    │             │             │
                    └─────────────┴─────────────┘
                                  │
                                  ▼
                          ┌──────────────┐
                          │Upload Assets │
                          └──────┬───────┘
                                 │
                                 ▼
                          ┌──────────────┐
                          │GitHub Release│
                          └──────────────┘
```

## Status Badges

Add these to README.md to show pipeline status:

```markdown
[![CI](https://github.com/lerian-studio/lerian-cli/actions/workflows/go-ci.yml/badge.svg)](https://github.com/lerian-studio/lerian-cli/actions/workflows/go-ci.yml)
[![Security](https://github.com/lerian-studio/lerian-cli/actions/workflows/go-security.yml/badge.svg)](https://github.com/lerian-studio/lerian-cli/actions/workflows/go-security.yml)
[![Coverage](https://github.com/lerian-studio/lerian-cli/actions/workflows/coverage.yml/badge.svg)](https://github.com/lerian-studio/lerian-cli/actions/workflows/coverage.yml)
[![Release](https://github.com/lerian-studio/lerian-cli/actions/workflows/go-release.yml/badge.svg)](https://github.com/lerian-studio/lerian-cli/actions/workflows/go-release.yml)
```

## Additional Resources

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [GoReleaser Documentation](https://goreleaser.com/)
- [Conventional Commits](https://www.conventionalcommits.org/)
- [Semantic Versioning](https://semver.org/)
- [Semantic Release](https://semantic-release.gitbook.io/)
- [GitFlow Workflow](https://www.atlassian.com/git/tutorials/comparing-workflows/gitflow-workflow)

## Questions?

If you have questions about the CI/CD pipeline:
1. Check this documentation first
2. Review workflow files in `.github/workflows/`
3. Check workflow run logs: `gh run list`
4. Open a discussion in GitHub Discussions
5. Contact the maintainers

---

**Last Updated:** 2025-11-26
**Version:** 3.0.0
**Maintainers:** Lerian Studio DevOps Team
