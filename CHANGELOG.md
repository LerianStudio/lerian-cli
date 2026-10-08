# Lerian-cli Changelog

## [1.3.0](https://github.com/LerianStudio/lerian-cli/releases/tag/v1.3.0)

Features:
- Developed a new feature branch and merged it into the main branch. (@bedatty)
- Added a new destination after a run and introduced a second movement for the banner. (@bedatty)
- Implemented a wizard beside the wordmark, providing a destination after a run. (@bedatty)
- Introduced a session with a new interface. (@bedatty)
- Offered commands when `lerian` is run without arguments. (@bedatty)
- Named the binary in the wordmark, illuminated it, and shortened the menu. (@bedatty)
- Allowed navigation with arrow keys where answers are predefined. (@bedatty)

Fixes:
- Bumped `golang.org/x/text` to address CVE-2026-56851. (@bedatty)
- Resolved issues with AWS credentials file requirements. (@bedatty)
- Fixed the menu to ensure reset is reachable and not the first option. (@bedatty)
- Corrected alignment of `needsChild` with offered options. (@bedatty)
- Removed a dead branch and aligned `needsChild` with available options. (@bedatty)
- Required a screen for the menu, not just a keyboard. (@bedatty)
- Ensured the menu is reachable from the menu and not the first item. (@bedatty)
- Made reset accessible from the menu. (@bedatty)
- Fixed three defects found during review, including a test issue. (@bedatty)
- Required a screen for the editor, maintained it, and ensured correct completion. (@bedatty)
- Corrected the application of a rule already stated in a marker comment. (@bedatty)

Improvements:
- Standardized the README on the `ungoliant-controller` structure and scoped it to `lerian-cli` with real usage previews. (@bedatty, @gandalf-at-lerian)
- Enhanced the README to align with the `ungoliant-controller` structure. (@gandalf-at-lerian)
- Improved the user interface by making the typed answer editable with Tab for path completion. (@bedatty)
- Enhanced the interface to offer commands when `lerian` is run without arguments. (@bedatty)
- Preallocated slices in loops for efficiency. (@bedatty)
- Standardized spelling to align with linter preferences. (@bedatty)
- Annotated the directory mode for reporting tools. (@bedatty)

[Compare changes](https://github.com/LerianStudio/lerian-cli/compare/v1.2.1...v1.3.0)

---

## [1.2.1](https://github.com/LerianStudio/lerian-cli/releases/tag/v1.2.1)

Fixes:
- Released `v1.2.1` with eight carried-over defects closed. (@bedatty)
- Stopped treating the operator's comments as section body. (@bedatty)
- Returned the original bytes for an equivalent section. (@bedatty)
- Made the credential lifetime visible and gateable before a write. (@bedatty)
- Ordered a single-service stage by its own product. (@bedatty)
- Reached the shared tier with `--set` and validated a typed egress address. (@bedatty)
- Kept Terraform JSON documents out of the unit logs. (@bedatty)
- Re-checked the credential at write time and named a usable remedy. (@bedatty)
- Matched `parseINI` and stripped inline comments before comparing. (@bedatty)
- Left an equivalent section byte for byte instead of rewriting it. (@bedatty)

Improvements:
- Bumped the Go dependencies group across one directory with six updates. (@bedatty)
- Corrected the schedule timezone and the change-gate rules in documentation. (@bedatty)
- Described the existing pipeline in documentation. (@bedatty)

Style:
- Spelled honoring the way the linter wants. (@bedatty)
- Wrapped the preflight guard call under the line limit. (@bedatty)
- Spelled canceled the way the linter wants. (@bedatty)

Test:
- Blocked every ambient credential source in `isolateAWS`. (@bedatty)

[Compare changes](https://github.com/LerianStudio/lerian-cli/compare/v1.2.0...v1.2.1)

---

## [1.2.0](https://github.com/LerianStudio/lerian-cli/releases/tag/v1.2.0)

Features:
- Release the unified CLI with infrastructure commands, integrating the `lerian-infra-cli` commands into the CLI as 'lerian infra'. (@bedatty)

Fixes:
- Release `v1.2.0` with binaries as proven by the beta. (@bedatty)
- Grant packages write permissions to allow the release caller to initiate the process. (@bedatty)
- Restore binary releases using the shared goreleaser lane. (@bedatty)
- Declare the multi-line error convention of the infra packages in golangci. (@bedatty)
- Satisfy repository lint requirements on the ported packages. (@bedatty)

Improvements:
- Correct the Go badge and update the ported package comment in the documentation. (@bedatty)

[Compare changes](https://github.com/LerianStudio/lerian-cli/compare/v1.1.1...v1.2.0)

---

## [1.1.1](https://github.com/LerianStudio/lerian-cli/releases/tag/v1.1.1)

Features:

- Test the GPT changelog generation process to ensure accurate and efficient changelog creation. (@maciell1)

Fixes:

- Align CI workflows with the shared workflows tier-1 boilerplate to maintain consistency and reduce errors. (@bedatty)

Improvements:

- Remove the legacy go-release caller and empty `.gitkeep` files from the CI process to streamline operations. (@bedatty)
- Refresh shared workflows to the latest tier-1 standards, ensuring up-to-date practices and tools are in use. (@bedatty)

[Compare changes](https://github.com/LerianStudio/lerian-cli/compare/v1.1.0...v1.1.1)

---

## [1.1.0](https://github.com/LerianStudio/lerian-cli/releases/tag/v1.1.0)

- **Features:**
  - Added build type detection helpers to enhance version management.

- **Improvements:**
  - Improved package and variable documentation for better code clarity.
  - Added package documentation to the command module.

- **Fixes:**
  - Fixed duplicate contributor entry in the changelog.
  - Removed duplicate v1.0.0 entry from the changelog.

Contributors: @ferr3ira-gabriel, @ferr3ira.gabriel, @lerian-studio-midaz-push-bot[bot]

[Compare changes](https://github.com/LerianStudio/lerian-cli/compare/v1.0.0...v1.1.0)

---

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


## Test Entry
- Test GPT changelog generation 2026-01-19

