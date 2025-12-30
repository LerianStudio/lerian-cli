# Lerian-cli Changelog

## [1.0.0](https://github.com/LerianStudio/lerian-cli/releases/tag/v1.0.0)

- **Features**
  - Added pr-security-scan workflow for CLI.
  - Introduced GPT Changelog workflow for AI-powered release notes.
  - Implemented semantic-release automation with CI/CD integration.

- **Fixes**
  - Removed deprecated rlcp field from goreleaser config.
  - Updated goreleaser config to version 2.
  - Corrected secret name in pr-validation workflow.
  - Skipped file permission test when running as root.
  - Disabled coverage comment in go-ci workflow.

- **Improvements**
  - Updated all workflows to use secrets inherit.
  - Enhanced installation documentation with an install script.
  - Consolidated workflows and upgraded Go to 1.25.
  - Updated shared workflows to v1.3.5.
  - Added standardized paths-ignore patterns.

Contributors: @ferr3ira-gabriel, @ferr3ira.gabriel

[View all changes](https://github.com/LerianStudio/lerian-cli/commits/v1.0.0)

---

## [1.0.0](https://github.com/LerianStudio/lerian-cli/releases/tag/v1.0.0)

- **Features:**
  - Added a CI gate workflow to ensure all checks pass before release.
  - Integrated semantic-release automation with CI/CD.
  - Added a pr-security-scan workflow for the CLI.
  - Introduced a GPT Changelog workflow for AI-powered release notes.
  - Upgraded Go version to 1.25.4 for improved performance and security.

- **Fixes:**
  - Updated GoReleaser config to version 2 and removed deprecated fields.
  - Corrected various workflow configurations, including secret management and trigger issues.
  - Resolved issues with golangci-lint configuration and violations.
  - Fixed Windows build issues in integration tests.
  - Addressed SARIF upload permission issues in security workflows.

- **Improvements:**
  - Updated shared workflows to the latest versions for better CI/CD integration.
  - Enhanced CI/CD documentation with ASCII diagrams and comprehensive guides.
  - Improved installation documentation with an install script.
  - Simplified CI/CD pipeline architecture for better efficiency.
  - Added comprehensive test coverage to ensure code quality.

Contributors: @ferr3ira-gabriel

[View all changes](https://github.com/LerianStudio/lerian-cli/commits/v1.0.0)

