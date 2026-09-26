# Lerian CLI — CI/CD

Three workflows, and all three are thin callers: the logic lives in
[`LerianStudio/github-actions-shared-workflows`](https://github.com/LerianStudio/github-actions-shared-workflows),
and this repository only declares what is specific to it.

| Workflow | Fires on | Calls |
|---|---|---|
| `pr-validation.yml` | pull request to `develop`, `release-candidate`, `main` | `go-pr-validation.yml@tier-1` |
| `release.yml` | push to `develop`, `release-candidate`, `main` | `go-release.yml@tier-1` |
| `routine.yml` | Monday 03:00 **UTC**, PR closed, push to `.github/labels.yml`, manual | `routine.yml@tier-1` |

## The tier channel

Every `uses:` points at `@tier-1`, a **branch** of the shared repository, never a version tag
and never a SHA.

That is deliberate and it is the portfolio convention. A release reaches this repository when
the channel is promoted upstream — `main → tier-0 → tier-1 → tier-2`, with human approval
between rings — not when someone opens a bump PR here. **The pin is set once and is not edited
again.** A PR that changes `@tier-1` to a version is moving the repository backwards.

`tier-1` is the ring for product repositories: it receives a stable release after `tier-0` has
carried it without incident.

SHA pinning still applies to third-party actions, which Dependabot keeps current. That is a
different lifecycle and is not channeled.

## PR validation

`go-pr-validation.yml` is an umbrella. One caller, several pipelines:

- **PR metadata** — Conventional Commits title, allowed scopes, auto-labeler, size label,
  breaking-change guard, signed-commit check, source-branch policy
- **Go analysis** — lint, tests, coverage, build
- **Security** — Trivy filesystem scan, prerelease checks
- **CodeRabbit gate** — asks for a review only once the validation is green, so no review is
  spent on a red or out-of-scope PR

What this repository sets, and why each one differs from the shared default:

| Input | Value | Why |
|---|---|---|
| `go_version` | `1.26` | matches the `go` directive in `go.mod` |
| `golangci_lint_version` | `v2.12.2` | the version `.golangci.yml` (schema v2) is written for |
| `coverage_threshold` | `55` | measured, not aspirational — see below |
| `allowed_source_branches` | `develop\|hotfix/*` | the portfolio default is `hotfix/*\|release-candidate`, and this repository has no `release-candidate` branch; without the override nothing could reach `main` |
| `enable_docker_scan` | `false` | there is no Dockerfile: this ships binaries |
| `run_lib_version_check` | `false` | `go.mod` declares no `LerianStudio/*` dependency |
| `app_name_prefix` | `lerian-cli` | namespaces the coverage and build artifacts |
| `shared_paths` | `go.mod`, `go.sum`, `cmd/`, `internal/`, `Makefile`, `.github/workflows/` | a change here runs the analysis even when no component matched |

Everything else is left at the shared default on purpose. An input that merely repeats the
default hides what is actually a decision of this repository, and freezes a value the shared
workflow may want to evolve.

### About the coverage threshold

`55` is where the repository is, measured — not where it should be. The gap is concentrated in
`cmd/midaz/ledger`, which has no tests. Raising the number is work in its own right; moving it
without writing tests only moves the lie.

## Release

`go-release.yml` runs semantic-release, then publishes binaries with GoReleaser from the
`.goreleaser.yml` at the repository root.

| Input | Value | Why |
|---|---|---|
| `enable_goreleaser` | `true` | this repository ships binaries |
| `enable_dockerhub`, `enable_ghcr` | `false` | no container image; with both off the shared workflow skips the container lane entirely |
| `enable_gitops_update` | `false` | a CLI has no GitOps target |
| `build_on_release` | `true` | runs GoReleaser in the same run that published the tag |
| `build_on_release_include_prerelease` | `true` | so the betas cut from `develop` carry binaries too |
| `enable_changelog` | only on `main` | the changelog belongs to stable releases |

### Why `build_on_release` rather than the tag trigger

The lane accepts either: a tag push, or the same run that just published the tag. The tag-push
path depends on a tag push firing a workflow, which does not happen reliably when the tag is
created by a GitHub App token — which is exactly how the shared semantic-release creates it.
The same-run path has no such dependency.

This is not theoretical. Restoring binary publication took three attempts, and two of them
failed silently:

1. A PR that only touched `.github/**` never ran the workflow it was fixing — `release.yml`
   has `.github/**` in its `paths-ignore`.
2. The next run died as `startup_failure`, with no job, no log and no annotation: the caller
   granted no `packages` scope, and three jobs of the shared workflow declare it. **GitHub
   validates the declared permissions of every job in a called workflow, including the jobs
   that will not run.**

If a release run dies before producing a job, the caller's `permissions:` block is the first
suspect.

## Branch flow

```
feature/* ──▶ develop ──▶ release-candidate ──▶ main
hotfix/*  ─────────────────────────────────────▶ main
```

| Branch | Release | Example |
|---|---|---|
| `develop` | prerelease `beta` | `v1.2.0-beta.2` |
| `release-candidate` | prerelease `rc` | `v1.2.0-rc.1` |
| `main` | stable | `v1.2.0` |

Declared in `.releaserc.yml`.

**`release-candidate` does not exist in this repository yet.** It is declared in
`.releaserc.yml` and it is why `allowed_source_branches` carries an override. When the branch
is created, the override should be removed so the repository follows the portfolio default
again.

## What runs where

- **A PR that touches only documentation or `.github` metadata** skips the analysis and
  security pipelines: the change gate (`non-doc-changes`) treats them as meta.
- **`.github/workflows/`, `.github/actions/` and `.github/scripts/` are the exception** — the
  gate counts them as code whatever the ignore globs say, so a PR that changes a workflow does
  run the pipelines that workflow configures.
- **A push that touches only `.github/**`, docs or Markdown cuts no release.** That is a
  different mechanism: the `paths-ignore` on `release.yml`'s own `push` trigger, which has
  nothing to do with the PR change gate above.
- **`routine.yml`** syncs `.github/labels.yml`, closes stale PRs and issues, cleans merged
  branches and old workflow runs. Run it by hand from the Actions tab: `workflow_dispatch`,
  pick a routine, and note that `dry_run` defaults to **true** there.

## Configuration files

| File | Owns |
|---|---|
| `.releaserc.yml` | semantic-release: the three branches, the `conventionalcommits` preset, the release rules |
| `.goreleaser.yml` | what the binaries are: `lerian` from `./cmd/lerian`, the platforms, the packages |
| `.golangci.yml` | lint, schema v2, including the exclusions the ported `internal/infra` packages need |
| `.coderabbit.yml` | review gated by CI: `auto_review.enabled: false`, with the gate asking for the review |
| `.github/labeler.yml` / `labels.yml` | the `area:` labels and their definitions; the second is synced by `routine.yml` |
| `.github/dependabot.yml` | `gomod` and `github-actions`, both targeting `develop` |

## Troubleshooting

**A release run dies as `startup_failure`.** Missing permission in the caller. Compare the
`permissions:` block against every job of the shared workflow being called — including the
ones that would be skipped.

**A workflow fix does not cut a release.** `release.yml` has `.github/**` in the `paths-ignore`
of its `push` trigger, so a change that only touches workflows never runs the release it is
fixing. Touch a path in `shared_paths` — a blank line in the `Makefile` is enough — in the same
PR. This is about the release trigger only: PR validation does run on a workflow change, since
the change gate counts `.github/workflows/` as code.

**A PR cannot be merged with every check green.** The `develop-rule` ruleset requires every
review thread to be *resolved*, not merely replied to. Replying to a CodeRabbit comment leaves
the thread open.

**Coverage fails on a PR that added no code.** The threshold is repository-wide, not diff-wide.
