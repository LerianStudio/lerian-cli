# GitHub Actions Workflow Consolidation Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use ring:executing-plans to implement this plan task-by-task.

**Goal:** Consolidate 7 duplicate GitHub Actions workflow files into 3 streamlined workflows that follow the platform repository pattern.

**Architecture:** Replace individual workflow files (go-ci.yml, go-security.yml, unit-tests.yml, coverage.yml) with a single `go-combined-analysis.yml` that calls the shared `go-pr-analysis.yml` workflow. Consolidate two release workflows (go-release.yml, semantic-release.yml) into a single `release.yml`. Keep `pr-validation.yml` for PR title validation.

**Tech Stack:**
- GitHub Actions
- Shared workflows from `LerianStudio/github-actions-shared-workflows@feature/go-workflows`
- Go 1.24
- golangci-lint v2.6.2

**Global Prerequisites:**
- Environment: macOS/Linux with git installed
- Tools: gh CLI, git
- Access: Write access to lerian-cli repository
- State: Working from `chore/consolidate-workflows` branch

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
cd /Users/ferr3ira/Documents/empresas/lerian-studio/projetos/midaz/applications/lerian-cli
git branch --show-current  # Expected: chore/consolidate-workflows
git status                  # Expected: clean working tree (or minimal changes)
ls .github/workflows/       # Expected: 7 workflow files
```

---

## Task 1: Create go-combined-analysis.yml

**Files:**
- Create: `.github/workflows/go-combined-analysis.yml`

**Prerequisites:**
- Files must exist: `.github/workflows/` directory

**Step 1: Create the new combined analysis workflow**

Create file `.github/workflows/go-combined-analysis.yml`:

```yaml
name: "Go Combined Analysis"

# Runs Go CI, Security, Tests, Coverage, and Build checks
# Uses centralized shared workflow for maintenance
# Single app mode (not monorepo) - uses filter_paths: '["."]'

on:
  pull_request:
    branches:
      - develop
      - release-candidate
      - main
    types:
      - opened
      - synchronize
      - reopened

jobs:
  go-analysis:
    name: Go Analysis
    uses: LerianStudio/github-actions-shared-workflows/.github/workflows/go-pr-analysis.yml@feature/go-workflows
    with:
      filter_paths: '["."]'
      path_level: 1
      app_name_prefix: "lerian-cli"
      go_version: "1.24"
      golangci_lint_version: "v2.6.2"
      golangci_lint_args: "--timeout=5m"
      coverage_threshold: 25
      fail_on_coverage_threshold: true
      enable_lint: true
      enable_security: true
      enable_tests: true
      enable_coverage: true
      enable_build: true
    secrets:
      manage_token: ${{ secrets.MANAGE_TOKEN }}
```

**Step 2: Verify file creation**

Run:
```bash
cat .github/workflows/go-combined-analysis.yml
```

**Expected output:**
```
name: "Go Combined Analysis"
...
```

**Step 3: Validate YAML syntax**

Run:
```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/go-combined-analysis.yml'))" && echo "YAML is valid"
```

**Expected output:**
```
YAML is valid
```

**If Task Fails:**

1. **File not created:**
   - Check: `ls .github/workflows/` (directory exists?)
   - Fix: Create directory first: `mkdir -p .github/workflows`

2. **YAML invalid:**
   - Check indentation (must use spaces, not tabs)
   - Verify all quotes are properly closed

3. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 2: Delete go-ci.yml (duplicated by go-combined-analysis.yml)

**Files:**
- Delete: `.github/workflows/go-ci.yml`

**Prerequisites:**
- Task 1 completed (go-combined-analysis.yml exists)

**Step 1: Verify file exists before deletion**

Run:
```bash
ls -la .github/workflows/go-ci.yml
```

**Expected output:** File listing showing go-ci.yml exists

**Step 2: Delete the file**

Run:
```bash
rm .github/workflows/go-ci.yml
```

**Step 3: Verify deletion**

Run:
```bash
ls .github/workflows/go-ci.yml 2>&1 || echo "File deleted successfully"
```

**Expected output:**
```
ls: .github/workflows/go-ci.yml: No such file or directory
File deleted successfully
```

**If Task Fails:**

1. **File doesn't exist:**
   - Check: `ls .github/workflows/` (maybe already deleted?)
   - Skip this task if already deleted

2. **Permission denied:**
   - Check file permissions: `ls -la .github/workflows/go-ci.yml`
   - Fix: `chmod 644 .github/workflows/go-ci.yml` then retry

---

## Task 3: Delete go-security.yml (duplicated by go-combined-analysis.yml)

**Files:**
- Delete: `.github/workflows/go-security.yml`

**Prerequisites:**
- Task 1 completed (go-combined-analysis.yml exists)

**Step 1: Verify file exists before deletion**

Run:
```bash
ls -la .github/workflows/go-security.yml
```

**Expected output:** File listing showing go-security.yml exists

**Step 2: Delete the file**

Run:
```bash
rm .github/workflows/go-security.yml
```

**Step 3: Verify deletion**

Run:
```bash
ls .github/workflows/go-security.yml 2>&1 || echo "File deleted successfully"
```

**Expected output:**
```
ls: .github/workflows/go-security.yml: No such file or directory
File deleted successfully
```

**If Task Fails:**
- Same as Task 2

---

## Task 4: Delete unit-tests.yml (duplicated by go-combined-analysis.yml)

**Files:**
- Delete: `.github/workflows/unit-tests.yml`

**Prerequisites:**
- Task 1 completed (go-combined-analysis.yml exists)

**Step 1: Verify file exists before deletion**

Run:
```bash
ls -la .github/workflows/unit-tests.yml
```

**Expected output:** File listing showing unit-tests.yml exists

**Step 2: Delete the file**

Run:
```bash
rm .github/workflows/unit-tests.yml
```

**Step 3: Verify deletion**

Run:
```bash
ls .github/workflows/unit-tests.yml 2>&1 || echo "File deleted successfully"
```

**Expected output:**
```
ls: .github/workflows/unit-tests.yml: No such file or directory
File deleted successfully
```

**If Task Fails:**
- Same as Task 2

---

## Task 5: Delete coverage.yml (duplicated by go-combined-analysis.yml)

**Files:**
- Delete: `.github/workflows/coverage.yml`

**Prerequisites:**
- Task 1 completed (go-combined-analysis.yml exists)

**Step 1: Verify file exists before deletion**

Run:
```bash
ls -la .github/workflows/coverage.yml
```

**Expected output:** File listing showing coverage.yml exists

**Step 2: Delete the file**

Run:
```bash
rm .github/workflows/coverage.yml
```

**Step 3: Verify deletion**

Run:
```bash
ls .github/workflows/coverage.yml 2>&1 || echo "File deleted successfully"
```

**Expected output:**
```
ls: .github/workflows/coverage.yml: No such file or directory
File deleted successfully
```

**If Task Fails:**
- Same as Task 2

---

## Task 6: Commit PR analysis workflow changes

**Prerequisites:**
- Tasks 1-5 completed

**Step 1: Stage changes**

Run:
```bash
git add .github/workflows/go-combined-analysis.yml
git add .github/workflows/go-ci.yml
git add .github/workflows/go-security.yml
git add .github/workflows/unit-tests.yml
git add .github/workflows/coverage.yml
```

**Step 2: Verify staged changes**

Run:
```bash
git status
```

**Expected output:**
```
Changes to be committed:
  new file:   .github/workflows/go-combined-analysis.yml
  deleted:    .github/workflows/coverage.yml
  deleted:    .github/workflows/go-ci.yml
  deleted:    .github/workflows/go-security.yml
  deleted:    .github/workflows/unit-tests.yml
```

**Step 3: Commit**

Run:
```bash
git commit -m "refactor(ci): consolidate PR analysis workflows into single go-combined-analysis.yml

- Add go-combined-analysis.yml that calls shared go-pr-analysis.yml workflow
- Remove go-ci.yml (functionality now in combined workflow)
- Remove go-security.yml (functionality now in combined workflow)
- Remove unit-tests.yml (functionality now in combined workflow)
- Remove coverage.yml (functionality now in combined workflow)

This reduces 4 separate workflows to 1, following platform repo pattern."
```

**Expected output:**
```
[chore/consolidate-workflows ...] refactor(ci): consolidate PR analysis workflows...
 5 files changed, ...
```

**If Task Fails:**

1. **Nothing to commit:**
   - Check: `git status` (are files staged?)
   - Fix: Re-run git add commands

2. **Pre-commit hook fails:**
   - Check error message
   - Fix any linting issues and retry

---

### Code Review Checkpoint 1

After Task 6, run code review before proceeding.

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use ring:requesting-code-review
   - All reviewers run simultaneously (ring-default:code-reviewer, ring-default:business-logic-reviewer, ring-default:security-reviewer)
   - Wait for all to complete

2. **Handle findings by severity (MANDATORY):**

**Critical/High/Medium Issues:**
- Fix immediately (do NOT add TODO comments for these severities)
- Re-run all 3 reviewers in parallel after fixes
- Repeat until zero Critical/High/Medium issues remain

**Low Issues:**
- Add `TODO(review):` comments in code at the relevant location
- Format: `TODO(review): [Issue description] (reported by [reviewer] on [date], severity: Low)`

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code at the relevant location
- Format: `FIXME(nitpick): [Issue description] (reported by [reviewer] on [date], severity: Cosmetic)`

3. **Proceed only when:**
   - Zero Critical/High/Medium issues remain
   - All Low issues have TODO(review): comments added
   - All Cosmetic issues have FIXME(nitpick): comments added

---

## Task 7: Create consolidated release.yml

**Files:**
- Create: `.github/workflows/release.yml` (new consolidated release workflow)

**Prerequisites:**
- Task 6 completed

**Step 1: Create the new release workflow**

Create file `.github/workflows/release.yml`:

```yaml
name: "Release"

# This workflow creates releases based on conventional commits
# Triggers on push to release branches
# Single app mode (no filter_paths = release from root)

on:
  push:
    branches:
      - main
      - develop
      - release-candidate

jobs:
  release:
    name: Create Release
    uses: LerianStudio/github-actions-shared-workflows/.github/workflows/release.yml@feature/go-workflows
    secrets:
      lerian_studio_push_bot_app_id: ${{ secrets.LERIAN_STUDIO_MIDAZ_PUSH_BOT_APP_ID }}
      lerian_studio_push_bot_private_key: ${{ secrets.LERIAN_STUDIO_MIDAZ_PUSH_BOT_PRIVATE_KEY }}
      lerian_ci_cd_user_gpg_key: ${{ secrets.LERIAN_CI_CD_USER_GPG_KEY }}
      lerian_ci_cd_user_gpg_key_password: ${{ secrets.LERIAN_CI_CD_USER_GPG_KEY_PASSWORD }}
      lerian_ci_cd_user_name: ${{ secrets.LERIAN_CI_CD_USER_NAME }}
      lerian_ci_cd_user_email: ${{ secrets.LERIAN_CI_CD_USER_EMAIL }}
```

**Step 2: Verify file creation**

Run:
```bash
cat .github/workflows/release.yml
```

**Expected output:**
```
name: "Release"
...
```

**Step 3: Validate YAML syntax**

Run:
```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))" && echo "YAML is valid"
```

**Expected output:**
```
YAML is valid
```

**If Task Fails:**

1. **File not created:**
   - Check: `ls .github/workflows/` (directory exists?)

2. **YAML invalid:**
   - Check indentation (must use spaces, not tabs)
   - Verify all quotes are properly closed

---

## Task 8: Delete go-release.yml (replaced by release.yml)

**Files:**
- Delete: `.github/workflows/go-release.yml`

**Prerequisites:**
- Task 7 completed (release.yml exists)

**Step 1: Verify file exists before deletion**

Run:
```bash
ls -la .github/workflows/go-release.yml
```

**Expected output:** File listing showing go-release.yml exists

**Step 2: Delete the file**

Run:
```bash
rm .github/workflows/go-release.yml
```

**Step 3: Verify deletion**

Run:
```bash
ls .github/workflows/go-release.yml 2>&1 || echo "File deleted successfully"
```

**Expected output:**
```
ls: .github/workflows/go-release.yml: No such file or directory
File deleted successfully
```

**If Task Fails:**
- Same as Task 2

**Note on go-release.yml removal:**
The `go-release.yml` workflow used `goreleaser` to build and publish releases when tags are pushed or releases are published. The new `release.yml` uses semantic-release which:
1. Automatically creates tags based on conventional commits
2. Creates GitHub releases with changelogs
3. The `.goreleaser.yml` file in the repo root will still be used if goreleaser is invoked manually or via a separate workflow later

If goreleaser-based releases are still needed (e.g., for cross-platform binary builds), consider adding a separate `goreleaser.yml` workflow that triggers on release creation. For now, we're consolidating to semantic-release only.

---

## Task 9: Delete semantic-release.yml (replaced by release.yml)

**Files:**
- Delete: `.github/workflows/semantic-release.yml`

**Prerequisites:**
- Task 7 completed (release.yml exists)

**Step 1: Verify file exists before deletion**

Run:
```bash
ls -la .github/workflows/semantic-release.yml
```

**Expected output:** File listing showing semantic-release.yml exists

**Step 2: Delete the file**

Run:
```bash
rm .github/workflows/semantic-release.yml
```

**Step 3: Verify deletion**

Run:
```bash
ls .github/workflows/semantic-release.yml 2>&1 || echo "File deleted successfully"
```

**Expected output:**
```
ls: .github/workflows/semantic-release.yml: No such file or directory
File deleted successfully
```

**If Task Fails:**
- Same as Task 2

---

## Task 10: Commit release workflow changes

**Prerequisites:**
- Tasks 7-9 completed

**Step 1: Stage changes**

Run:
```bash
git add .github/workflows/release.yml
git add .github/workflows/go-release.yml
git add .github/workflows/semantic-release.yml
```

**Step 2: Verify staged changes**

Run:
```bash
git status
```

**Expected output:**
```
Changes to be committed:
  new file:   .github/workflows/release.yml
  deleted:    .github/workflows/go-release.yml
  deleted:    .github/workflows/semantic-release.yml
```

**Step 3: Commit**

Run:
```bash
git commit -m "refactor(ci): consolidate release workflows into single release.yml

- Add release.yml that calls shared release workflow
- Remove go-release.yml (goreleaser-based release on tag push)
- Remove semantic-release.yml (duplicate semantic release workflow)

Uses semantic-release for automated versioning based on conventional commits.
Single app mode (no filter_paths) releases from repository root."
```

**Expected output:**
```
[chore/consolidate-workflows ...] refactor(ci): consolidate release workflows...
 3 files changed, ...
```

**If Task Fails:**
- Same as Task 6

---

### Code Review Checkpoint 2

After Task 10, run code review before proceeding.

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use ring:requesting-code-review
   - All reviewers run simultaneously

2. **Handle findings by severity (MANDATORY):**
   - Same process as Code Review Checkpoint 1

3. **Proceed only when:**
   - Zero Critical/High/Medium issues remain
   - All Low/Cosmetic issues have appropriate TODO/FIXME comments

---

## Task 11: Verify pr-validation.yml (keep as-is)

**Files:**
- Verify: `.github/workflows/pr-validation.yml` (no changes needed)

**Prerequisites:**
- Tasks 1-10 completed

**Step 1: Verify pr-validation.yml still exists**

Run:
```bash
cat .github/workflows/pr-validation.yml | head -20
```

**Expected output:**
```yaml
name: PR Validation

on:
  pull_request:
    branches: [develop, release-candidate, main]
    types: [opened, synchronize, reopened, ready_for_review]
...
```

**Step 2: List remaining workflows**

Run:
```bash
ls -la .github/workflows/
```

**Expected output:** Should show exactly 3 files:
```
go-combined-analysis.yml
pr-validation.yml
release.yml
```

**If Task Fails:**

1. **pr-validation.yml missing:**
   - Check git history: `git log --oneline --all -- .github/workflows/pr-validation.yml`
   - Restore if accidentally deleted: `git checkout HEAD -- .github/workflows/pr-validation.yml`

---

## Task 12: Create labeler.yml configuration (if missing)

**Files:**
- Create: `.github/labeler.yml` (only if doesn't exist)

**Prerequisites:**
- Task 11 completed

**Step 1: Check if labeler.yml exists**

Run:
```bash
ls .github/labeler.yml 2>&1 || echo "File does not exist"
```

**If file exists:** Skip to Task 13

**If file does not exist:** Continue with Step 2

**Step 2: Create labeler.yml**

Create file `.github/labeler.yml`:

```yaml
# Label configuration for PR auto-labeling
# Used by pr-validation.yml workflow

# Code areas
cli:
  - changed-files:
      - any-glob-to-any-file: 'cmd/**/*'

internal:
  - changed-files:
      - any-glob-to-any-file: 'internal/**/*'

config:
  - changed-files:
      - any-glob-to-any-file: 'configs/**/*'

# File types
documentation:
  - changed-files:
      - any-glob-to-any-file: '**/*.md'
      - any-glob-to-any-file: 'docs/**/*'

tests:
  - changed-files:
      - any-glob-to-any-file: '**/*_test.go'

ci:
  - changed-files:
      - any-glob-to-any-file: '.github/**/*'

dependencies:
  - changed-files:
      - any-glob-to-any-file: 'go.mod'
      - any-glob-to-any-file: 'go.sum'
```

**Step 3: Verify file creation**

Run:
```bash
cat .github/labeler.yml
```

**Step 4: Commit if created**

Run:
```bash
git add .github/labeler.yml
git commit -m "chore(ci): add labeler configuration for PR auto-labeling"
```

**If Task Fails:**

1. **Directory doesn't exist:**
   - Fix: `mkdir -p .github`

---

## Task 13: Final verification

**Prerequisites:**
- All previous tasks completed

**Step 1: List all workflow files**

Run:
```bash
ls -la .github/workflows/
```

**Expected output:** Exactly 3 workflow files:
```
go-combined-analysis.yml
pr-validation.yml
release.yml
```

**Step 2: Validate all YAML files**

Run:
```bash
for f in .github/workflows/*.yml; do
  echo "Validating $f..."
  python3 -c "import yaml; yaml.safe_load(open('$f'))" && echo "  ✓ Valid"
done
```

**Expected output:**
```
Validating .github/workflows/go-combined-analysis.yml...
  ✓ Valid
Validating .github/workflows/pr-validation.yml...
  ✓ Valid
Validating .github/workflows/release.yml...
  ✓ Valid
```

**Step 3: Verify git status**

Run:
```bash
git status
```

**Expected output:**
```
On branch chore/consolidate-workflows
nothing to commit, working tree clean
```

Or if there are uncommitted changes, review and commit them.

**Step 4: View commit history**

Run:
```bash
git log --oneline -5
```

**Expected output:** Shows your consolidation commits

**If Task Fails:**

1. **Missing workflow files:**
   - Check git log to see if accidentally deleted
   - Restore from earlier commits if needed

2. **YAML validation fails:**
   - Fix the syntax errors in the failing file

---

### Code Review Checkpoint 3 (Final)

After Task 13, run final code review.

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use ring:requesting-code-review
   - All reviewers run simultaneously

2. **Handle findings by severity (MANDATORY):**
   - Same process as previous checkpoints

3. **Complete only when:**
   - Zero Critical/High/Medium issues remain
   - All changes are committed
   - Working tree is clean

---

## Task 14: Push branch and create PR

**Prerequisites:**
- All previous tasks completed
- All code reviews passed
- Working tree clean

**Step 1: Push branch**

Run:
```bash
git push -u origin chore/consolidate-workflows
```

**Expected output:**
```
...
To github.com:LerianStudio/lerian-cli.git
 * [new branch]      chore/consolidate-workflows -> chore/consolidate-workflows
Branch 'chore/consolidate-workflows' set up to track remote branch 'chore/consolidate-workflows' from 'origin'.
```

**Step 2: Create PR**

Run:
```bash
gh pr create --title "refactor(ci): consolidate GitHub Actions workflows" --body "## Summary

Consolidates 7 GitHub Actions workflow files into 3 streamlined workflows following the platform repository pattern.

## Changes

### PR Analysis Workflows (4 → 1)
- **Added:** \`go-combined-analysis.yml\` - Single workflow calling shared \`go-pr-analysis.yml\`
- **Removed:** \`go-ci.yml\` (lint, build, tests)
- **Removed:** \`go-security.yml\` (security scanning)
- **Removed:** \`unit-tests.yml\` (duplicate tests)
- **Removed:** \`coverage.yml\` (coverage check)

### Release Workflows (2 → 1)
- **Added:** \`release.yml\` - Single semantic release workflow
- **Removed:** \`go-release.yml\` (goreleaser-based release)
- **Removed:** \`semantic-release.yml\` (duplicate semantic release)

### Kept
- \`pr-validation.yml\` - PR title validation (unchanged)

## Benefits

1. **Reduced complexity:** 7 files → 3 files
2. **Centralized maintenance:** Uses shared workflows from \`github-actions-shared-workflows\`
3. **Consistent pattern:** Matches platform repository structure
4. **Single source of truth:** No duplicate CI checks

## Testing

- [ ] PR analysis workflow triggers on PR events
- [ ] Release workflow triggers on push to main/develop/release-candidate
- [ ] PR validation workflow still validates PR titles

## Configuration

The new \`go-combined-analysis.yml\` uses these settings:
- Go version: 1.24
- golangci-lint version: v2.6.2
- Coverage threshold: 25% (fail on threshold)
- All checks enabled: lint, security, tests, coverage, build
" --base main
```

**Expected output:**
```
https://github.com/LerianStudio/lerian-cli/pull/XX
```

**If Task Fails:**

1. **Push rejected:**
   - Check: Do you have write access?
   - Fix: Contact repository admin

2. **PR creation fails:**
   - Check: Is gh CLI authenticated?
   - Fix: `gh auth login`

---

## Summary

After completing all tasks, you will have:

| Before (7 files) | After (3 files) |
|------------------|-----------------|
| go-ci.yml | go-combined-analysis.yml |
| go-security.yml | (merged into above) |
| unit-tests.yml | (merged into above) |
| coverage.yml | (merged into above) |
| go-release.yml | release.yml |
| semantic-release.yml | (merged into above) |
| pr-validation.yml | pr-validation.yml (unchanged) |

**Workflow Mapping:**

1. **go-combined-analysis.yml** (NEW)
   - Triggers: PR to develop/release-candidate/main
   - Runs: lint, security, tests, coverage, build
   - Replaces: go-ci.yml, go-security.yml, unit-tests.yml, coverage.yml

2. **release.yml** (NEW)
   - Triggers: Push to main/develop/release-candidate
   - Runs: Semantic release with conventional commits
   - Replaces: go-release.yml, semantic-release.yml

3. **pr-validation.yml** (UNCHANGED)
   - Triggers: PR events
   - Runs: PR title/description validation, auto-labeling

---

## Plan Checklist

- [x] Header with goal, architecture, tech stack, prerequisites
- [x] Verification commands with expected output
- [x] Tasks broken into bite-sized steps (2-5 min each)
- [x] Exact file paths for all files
- [x] Complete code (no placeholders)
- [x] Exact commands with expected output
- [x] Failure recovery steps for each task
- [x] Code review checkpoints after batches
- [x] Severity-based issue handling documented
- [x] Passes Zero-Context Test
