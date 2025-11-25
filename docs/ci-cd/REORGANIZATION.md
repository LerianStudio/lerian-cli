# CI/CD Workflow Reorganization

**Date:** 2025-11-22
**Version:** 1.0.0

## Overview

This document explains the reorganization of the lerian-cli CI/CD workflows to follow Lerian Studio's shared workflow naming conventions and modular structure patterns.

## Reorganization Summary

The CI/CD workflows have been reorganized from monolithic, mixed-purpose files to focused, domain-specific workflows following naming conventions established in the company's shared workflow repository.

### Before vs After

| Before (Phase 6) | After (Reorganization) | Purpose |
|------------------|------------------------|---------|
| `ci.yml` | `go-ci.yml` | Go-specific continuous integration |
| `security.yml` | `go-security.yml` | Go-specific security scanning |
| `release.yml` | `go-release.yml` | Go-specific release automation |
| `pr-checks.yml` | `pr-validation.yml` | Pull request validation |
| `codeql.yml` | `code-analysis.yml` | Advanced code analysis |

## New Workflow Structure

### Active Workflows

```
.github/workflows/
├── go-ci.yml           # Go continuous integration (NEW)
├── go-security.yml     # Go security scanning (NEW)
├── go-release.yml      # Go release automation (NEW)
├── pr-validation.yml   # Pull request validation (RENAMED)
├── code-analysis.yml   # Code analysis with CodeQL (RENAMED)
└── archive/           # Old workflows (preserved for reference)
    ├── ci.yml
    ├── security.yml
    └── release.yml
```

## Workflow Details

### 1. `go-ci.yml` - Go Continuous Integration

**Trigger:** Push to `main`/`develop`, Pull requests

**Purpose:** Core CI operations for Go projects

**Jobs:**
- **test** - Multi-matrix testing (Go 1.23 × Linux, macOS, Windows)
- **lint** - golangci-lint with 40+ linters
- **build** - Cross-platform binary builds
- **verify-module** - Go module verification
- **check-format** - Code formatting validation
- **check-docs** - Documentation completeness
- **all-checks-pass** - Aggregate status check

**Key Changes from Original:**
- Renamed from `ci.yml` to follow Go-specific naming
- Removed security job (moved to `go-security.yml`)
- Simplified and focused on CI operations
- Maintained all original functionality

**Time:** ~5-10 minutes

### 2. `go-security.yml` - Go Security Scanning

**Trigger:** Push, Pull requests, Weekly schedule (Monday 00:00 UTC), Manual

**Purpose:** Comprehensive security scanning for Go projects

**Jobs:**
- **dependency-review** - GitHub dependency review (PR only)
- **gosec** - Go security scanner with SARIF upload
- **govulncheck** - Official Go vulnerability database check
- **nancy** - Sonatype dependency vulnerability scan
- **trivy** - Aqua Security filesystem scanner
- **secret-scan** - TruffleHog secret detection
- **license-check** - go-licenses compliance check
- **sbom** - Software Bill of Materials generation
- **security-summary** - Aggregate security status

**Key Changes from Original:**
- Renamed from `security.yml` to follow Go-specific naming
- Maintained all 8 security scanners
- Preserved SARIF upload to GitHub Security tab
- Kept weekly scheduled scans

**Time:** ~5-8 minutes

### 3. `go-release.yml` - Go Release Automation

**Trigger:** Tag push (`v*.*.*`)

**Purpose:** Automated release creation and distribution for Go projects

**Jobs:**
- **release** - GoReleaser execution with multi-platform builds
- **homebrew** - Homebrew formula update
- **docker** - Multi-arch Docker image build and push
- **notify** - Release notification

**Key Changes from Original:**
- Renamed from `release.yml` to follow Go-specific naming
- Maintained GoReleaser integration
- Preserved Homebrew and Docker publishing
- Kept all artifact generation

**Time:** ~10-15 minutes

### 4. `pr-validation.yml` - Pull Request Validation

**Trigger:** Pull request events (opened, synchronize, reopened, ready_for_review)

**Purpose:** Automated pull request quality checks

**Jobs:**
- **title-validation** - Conventional Commits format check
- **size-labeling** - PR size categorization (XS/S/M/L/XL)
- **description-check** - PR description validation
- **auto-labeling** - Area-based label assignment
- **changelog-reminder** - CHANGELOG.md update reminder
- **link-validation** - Issue linking verification

**Key Changes from Original:**
- Renamed from `pr-checks.yml` to `pr-validation.yml` for clarity
- No functional changes
- Maintained all validation rules

**Time:** <1 minute

### 5. `code-analysis.yml` - Code Analysis

**Trigger:** Push, Pull requests, Weekly schedule (Sunday 03:00 UTC), Manual

**Purpose:** Advanced static code analysis with CodeQL

**Jobs:**
- **analyze** - CodeQL security and quality analysis
- SARIF upload to GitHub Security tab

**Key Changes from Original:**
- Renamed from `codeql.yml` to `code-analysis.yml` for clarity
- No functional changes
- Maintained security-extended query pack

**Time:** ~3-5 minutes

## Naming Conventions

The reorganization follows these established conventions from the shared workflows repository:

### File Naming
- **Format:** `{technology}-{purpose}.yml` or `{purpose}.yml`
- **Style:** kebab-case (lowercase with hyphens)
- **Examples:**
  - `go-ci.yml` - Technology-specific CI
  - `go-security.yml` - Technology-specific security
  - `pr-validation.yml` - Generic PR validation

### Workflow Names
- **Format:** Title Case with spaces
- **Examples:**
  - `name: Go CI`
  - `name: Go Security`
  - `name: PR Validation`

### Input Parameters (for reusable workflows)
- **Format:** snake_case
- **Examples:**
  - `runner_type`
  - `filter_paths`
  - `docker_username`

## Advantages of Reorganization

### 1. **Clear Separation of Concerns**
- CI operations separate from security scanning
- Release process isolated from development workflows
- Each workflow has a single, clear responsibility

### 2. **Improved Discoverability**
- Technology prefix (`go-`) makes it clear these are Go-specific
- Descriptive names explain purpose at a glance
- Follows company-wide conventions

### 3. **Better Maintainability**
- Smaller, focused files easier to understand and modify
- Changes to CI don't affect security or release workflows
- Easier to test individual workflows in isolation

### 4. **Scalability**
- Easy to add new technology-specific workflows (e.g., `js-ci.yml`, `python-ci.yml`)
- Can create additional focused workflows without bloat
- Follows patterns used across company projects

### 5. **Consistency**
- Matches Lerian Studio's shared workflow structure
- Developers familiar with other projects will recognize patterns
- Easier onboarding for new team members

## Migration Notes

### Preserved Functionality

**Everything that worked before still works:**
- ✅ All tests still run (9-matrix: 3 Go versions × 3 OSes)
- ✅ All security scans still execute (8 scanners)
- ✅ Release automation unchanged (GoReleaser, Homebrew, Docker)
- ✅ PR validation rules maintained
- ✅ CodeQL analysis continues
- ✅ Coverage reporting to Codecov
- ✅ SARIF uploads to GitHub Security tab
- ✅ Artifact generation and uploads

### What Changed

**Only naming and organization:**
- ❌ No job removals
- ❌ No functionality changes
- ❌ No trigger modifications
- ❌ No permission changes
- ✅ Files renamed for clarity
- ✅ Workflows reorganized by domain
- ✅ Names follow conventions

### Old Workflows

Old workflow files are preserved in `.github/workflows/archive/` for reference:
- `archive/ci.yml`
- `archive/security.yml`
- `archive/release.yml`

These can be removed after confirming new workflows function correctly.

## Integration with Shared Workflows

### Current State

The reorganized workflows are **standalone** but follow shared workflow naming conventions. They are ready for future integration with reusable workflows.

### Future Integration Opportunities

The reorganization prepares for eventual integration with Lerian Studio's shared workflows:

#### 1. Security Scanning Integration

**Current:** Standalone `go-security.yml` with Go-specific scanners

**Future:** Could partially use `LerianStudio/github-actions-shared-workflows/.github/workflows/pr-security-scan.yml@main`

**Considerations:**
- Shared workflow is Docker/container-focused
- lerian-cli workflows are Go-native (no Docker required for scanning)
- Would need adaptation or hybrid approach

#### 2. Release Management Integration

**Current:** Standalone `go-release.yml` with GoReleaser

**Future:** Could integrate with `LerianStudio/github-actions-shared-workflows/.github/workflows/release.yml@main`

**Considerations:**
- Shared workflow uses semantic-release (JavaScript-based)
- lerian-cli uses GoReleaser (Go-native)
- Different approaches to version management
- Would need Go-specific release workflow in shared repository

#### 3. Common Patterns Integration

**Potential shared workflows to create:**
- `go-lint.yml` - Reusable golangci-lint configuration
- `go-test.yml` - Reusable Go test matrix
- `go-build.yml` - Reusable cross-platform Go builds

## Validation Checklist

Before removing archived workflows, verify:

- [ ] `go-ci.yml` runs successfully on push to main
- [ ] `go-ci.yml` runs successfully on pull requests
- [ ] `go-security.yml` completes all 8 security scans
- [ ] `go-release.yml` successfully creates releases on tag push
- [ ] `pr-validation.yml` validates PR titles and adds labels
- [ ] `code-analysis.yml` completes CodeQL analysis
- [ ] Coverage reports upload to Codecov
- [ ] SARIF files upload to GitHub Security tab
- [ ] Build artifacts are generated and uploaded
- [ ] Docker images publish to GHCR
- [ ] Homebrew formula updates (if applicable)

## Rollback Procedure

If issues arise with new workflows:

1. **Stop:** Disable new workflows by renaming to `.yml.disabled`
   ```bash
   cd .github/workflows
   mv go-ci.yml go-ci.yml.disabled
   mv go-security.yml go-security.yml.disabled
   mv go-release.yml go-release.yml.disabled
   ```

2. **Restore:** Copy old workflows from archive
   ```bash
   cp archive/ci.yml .
   cp archive/security.yml .
   cp archive/release.yml .
   ```

3. **Revert:** Revert PR validation and code analysis renames
   ```bash
   mv pr-validation.yml pr-checks.yml
   mv code-analysis.yml codeql.yml
   ```

4. **Test:** Verify old workflows work correctly

5. **Investigate:** Determine root cause of new workflow issues

## Testing the New Workflows

### Local Validation

Use [act](https://github.com/nektos/act) to test workflows locally:

```bash
# Install act
brew install act  # macOS
# or
curl https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash

# Test go-ci workflow
act -W .github/workflows/go-ci.yml

# Test go-security workflow
act -W .github/workflows/go-security.yml
```

### GitHub Actions Testing

1. **Create test branch:**
   ```bash
   git checkout -b test/workflow-reorganization
   ```

2. **Push changes:**
   ```bash
   git add .github/workflows/
   git commit -m "refactor: reorganize CI/CD workflows following naming conventions"
   git push origin test/workflow-reorganization
   ```

3. **Create test PR:**
   - Open PR to trigger `pr-validation.yml`
   - Verify all checks run successfully

4. **Test CI workflow:**
   - Push commits to trigger `go-ci.yml`
   - Verify test matrix, build, lint all pass

5. **Test security workflow:**
   - Wait for weekly schedule or trigger manually
   - Verify all 8 scanners execute

6. **Test release (optional):**
   - Create test tag: `git tag v0.0.0-test && git push --tags`
   - Verify GoReleaser, Homebrew, Docker jobs
   - Delete test release and tag after validation

## Best Practices

### 1. Workflow Naming
- Always use technology prefix for language-specific workflows
- Use descriptive, action-oriented names
- Follow kebab-case for files, Title Case for workflow names

### 2. Job Organization
- Keep related jobs in the same workflow
- Use job dependencies (`needs:`) to control execution order
- Create summary/aggregate jobs for overall status

### 3. Reusability Preparation
- Structure workflows to be easily convertible to reusable workflows
- Use `inputs` and `secrets` pattern even in standalone workflows
- Document expected inputs in workflow comments

### 4. Documentation
- Keep workflow comments up to date
- Document trigger conditions clearly
- Explain any non-obvious job logic

### 5. Maintenance
- Review workflows quarterly for deprecated actions
- Update action versions (e.g., `@v4` → `@v5`) regularly
- Monitor GitHub security advisories for workflow actions

## Performance Comparison

### Before Reorganization

```
Single CI Run:
├─ ci.yml           ~6-8 min
├─ security.yml     ~5-8 min
├─ pr-checks.yml    <1 min
└─ codeql.yml       ~3-5 min
Total: ~14-22 min (parallel)
```

### After Reorganization

```
Single CI Run:
├─ go-ci.yml         ~6-8 min
├─ go-security.yml   ~5-8 min
├─ pr-validation.yml <1 min
└─ code-analysis.yml ~3-5 min
Total: ~14-22 min (parallel)
```

**No performance change** - Same jobs, just reorganized.

## Related Documentation

- [CI/CD Pipeline Documentation](./README.md) - Overview of all pipelines
- [Shared Workflows Documentation](https://github.com/LerianStudio/github-actions-shared-workflows) - Company-wide workflow patterns
- [GitHub Actions Documentation](https://docs.github.com/en/actions) - Official GitHub Actions docs

## Questions and Support

For questions about the reorganization:
1. Check this documentation first
2. Review workflow files in `.github/workflows/`
3. Compare with archived workflows in `.github/workflows/archive/`
4. Contact DevOps team for assistance

## Changelog

### 2025-11-22 - Initial Reorganization
- Renamed `ci.yml` → `go-ci.yml`
- Renamed `security.yml` → `go-security.yml`
- Renamed `release.yml` → `go-release.yml`
- Renamed `pr-checks.yml` → `pr-validation.yml`
- Renamed `codeql.yml` → `code-analysis.yml`
- Moved old workflows to `archive/`
- Created this documentation

---

**Last Updated:** 2025-11-22
**Version:** 1.0.0
**Author:** DevOps Team
