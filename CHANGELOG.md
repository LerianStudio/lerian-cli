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

Contributors: @ferr3ira-gabriel

[View all changes](https://github.com/LerianStudio/lerian-cli/commits/v1.0.0)
