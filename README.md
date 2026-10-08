<table border="0" cellspacing="0" cellpadding="0">
  <tr>
    <td><img src="https://github.com/LerianStudio.png" width="72" alt="Lerian" /></td>
    <td><h1>lerian-cli</h1></td>
  </tr>
</table>

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Cobra](https://img.shields.io/badge/CLI-Cobra-6558F5)](https://github.com/spf13/cobra)
[![Terraform](https://img.shields.io/badge/Terraform-infra-844FBA?logo=terraform&logoColor=white)](https://www.terraform.io/)
[![Status](https://img.shields.io/badge/status-beta-orange)](#)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](./LICENSE)

Official command-line interface for the Lerian platform. One binary, `lerian`, to sign in, deploy AWS infrastructure from the Lerian Terraform templates, and manage Midaz ledger deployments.

> **Status: beta.** Releases are published from `develop` as `-beta` and from `release-candidate` as `-rc`. Command names and flags can still change. See [`CHANGELOG.md`](./CHANGELOG.md) for what moved.

## 🎯 Purpose

Give every Lerian operator one tool for the repeatable parts of running the platform. It replaces the separate `lerian-infra` binary and the `deploy.sh` script behind it, and it is where product commands (Midaz today, others later) live. It prompts when run in a terminal and never prompts in CI, so the same command works for a person and for a pipeline.

## 🧩 What it does (today)

| Command group | What it does | Reference |
|---|---|---|
| `lerian` (no arguments) | Opens an interactive session: pick a command, run it, pick the next one | [`docs/interactive-session.md`](./docs/interactive-session.md) |
| `lerian auth` | Signs in to the Lerian platform (not AWS) with a named profile | [Authentication](#authentication) |
| `lerian infra` | Drives the Terraform roots of [lerian-terraform-foundation](https://github.com/LerianStudio/lerian-terraform-foundation) on AWS, from bootstrap to per-product services | [`docs/infra.md`](./docs/infra.md) |
| `lerian config` | Shows or resets what the CLI remembers on this machine | [`docs/infra.md`](./docs/infra.md#what-the-cli-remembers-and-how-to-forget-it) |
| `lerian midaz ledger` | Creates, lists, describes and deletes Midaz ledger deployments, plus logs, port-forward, SQL, backup and events | [`docs/midaz.md`](./docs/midaz.md) |
| `lerian version` | Prints build information | — |

Flowker, Reporter, Tracer and Fees are planned and not available yet.

Safety properties worth knowing before you run `infra`:

- **Account guard.** Three checks run before anything touches AWS, with no flag to bypass them.
- **Dry run.** `--dry-run` resolves and prints the whole execution plan without a single AWS call.
- **Output formats.** `table`, `json` and `yaml` through `--output`.
- **CI-safe.** Outside a terminal nothing is ever asked; a missing flag is named in the error.

## 📁 Directory layout

```
.
├── README.md                         # this file
├── cmd/
│   ├── lerian/                       # main entrypoint
│   ├── auth/                         # lerian auth login / logout
│   ├── infra/                        # lerian infra
│   └── midaz/ledger/                 # lerian midaz ledger ...
├── internal/
│   ├── client/                       # HTTP client for the Lerian API
│   ├── config/                       # ~/.lerian/config.yaml, profiles, reset
│   ├── infra/                        # Terraform orchestration, account guard, tfvars
│   ├── infracli/                     # terminal face of infra: wizard, prompts, check, init
│   ├── kubectl/                      # Kubernetes operations used by midaz commands
│   ├── output/                       # table / json / yaml printer
│   └── version/                      # build identity
├── docs/                             # command references, CI/CD, testing strategy
├── scripts/install.sh                # release installer
├── Makefile
├── .goreleaser.yml / .releaserc.yml  # release pipeline
└── .github/workflows/                # pr-validation, release, routine
```

## 🚀 Install

The installer downloads the latest release through the GitHub CLI, so it needs [`gh`](https://cli.github.com/) installed and authenticated with access to this repository.

```bash
curl -fsSL https://raw.githubusercontent.com/LerianStudio/lerian-cli/main/scripts/install.sh | sh
```

Options:

```bash
# Specific version
curl -fsSL https://raw.githubusercontent.com/LerianStudio/lerian-cli/main/scripts/install.sh | sh -s -- --version v1.0.0

# Custom install directory (default: ~/.local/bin)
curl -fsSL https://raw.githubusercontent.com/LerianStudio/lerian-cli/main/scripts/install.sh | INSTALL_DIR=/usr/local/bin sh
```

Verify:

```bash
lerian version
```

<details>
<summary><strong>Other installation methods</strong></summary>

**Manual download**

```bash
# Example for Linux amd64
gh release download --repo LerianStudio/lerian-cli --pattern "lerian_*_Linux_x86_64.tar.gz"
tar -xzf lerian_*_Linux_x86_64.tar.gz
sudo mv lerian /usr/local/bin/
```

**Linux packages**

```bash
# Debian/Ubuntu
gh release download --repo LerianStudio/lerian-cli --pattern "*.deb"
sudo dpkg -i lerian_*.deb

# RHEL/Fedora
gh release download --repo LerianStudio/lerian-cli --pattern "*.rpm"
sudo rpm -i lerian_*.rpm
```

**Go install** — requires Go 1.26+ and access to the private repository via `GOPRIVATE`.

```bash
go install github.com/LerianStudio/lerian-cli/cmd/lerian@latest
```

**From source** — see Development below.

</details>

<details>
<summary><strong>Shell completions</strong></summary>

```bash
# Bash
lerian completion bash > /etc/bash_completion.d/lerian

# Zsh
lerian completion zsh > "${fpath[1]}/_lerian"

# Fish
lerian completion fish > ~/.config/fish/completions/lerian.fish

# PowerShell
lerian completion powershell > lerian.ps1
```

</details>

## ▶️ Usage

### Global flags

- `--config` — config file path (default: `$HOME/.lerian/config.yaml`)
- `--profile, -p` — Lerian platform profile to use (default: `default`). Not an AWS profile; `lerian infra` has its own.
- `--output, -o` — output format: `table`, `json`, `yaml` (default: `table`)
- `--help, -h` — help for any command
- `--version, -v` — version information

### Authentication

```bash
lerian auth login \
  --api-url https://api.lerian.studio \
  --api-key YOUR_API_KEY \
  --tenant-id YOUR_TENANT_ID

# Named profile
lerian auth login --profile production --api-url ... --api-key ... --tenant-id ...

lerian auth logout
```

### Infrastructure

```bash
# Verify this machine: dependencies, checkout, what it would use (no AWS call)
lerian infra check

# Write the configuration a fresh checkout needs
lerian infra init --env dev

# Resolve and print the execution plan, touching nothing
lerian infra --env dev --target all --dry-run

# Stand up an environment, in order
lerian infra --env dev --target bootstrap  --action apply
lerian infra --env dev --target infra-base --action apply

# Read the helm values of a product back out
lerian infra --env dev --target midaz --action helm-values --format yaml
```

Run with no `--env` in a terminal and it asks instead. `lerian infra --help` has every flag, the ordering rules and the environment variables it reads. Full walkthrough: [`docs/infra.md`](./docs/infra.md).

### Midaz

```bash
lerian midaz ledger create --name my-ledger --region us-east-1 --env dev
lerian midaz ledger list -o json
lerian midaz ledger describe <ledger-id>
lerian midaz ledger logs <ledger-id> --follow
```

Deployment modes (SaaS, private, sandbox), regions, sizes and every operation: [`docs/midaz.md`](./docs/midaz.md).

### Configuration

Everything the CLI remembers lives in one file, `~/.lerian/config.yaml`: where the templates checkout is, and the profiles `lerian auth login` creates.

```bash
lerian config          # what it has written down, and where
lerian config reset    # forget it, as if the CLI had never run here
```

```yaml
current-profile: default
profiles:
  default:
    api-url: https://api.lerian.studio
    api-key: ***
    tenant-id: your-tenant-uuid-here
  production:
    api-url: https://api.lerian.studio
    api-key: ***
    tenant-id: prod-tenant-uuid
```

`reset` removes that file and nothing else. `~/.aws` and the templates checkout are never touched unless you ask for the checkout to be deleted.

### Environment variables

- `LERIAN_TF_REPO` — path to the lerian-terraform-foundation checkout, for this shell.
- `LERIAN_NO_BANNER=1` — turns the session banner off.
- `NO_COLOR`, `TERM=dumb` — honored throughout; no color, no animation.

## 🔧 Troubleshooting

**Cannot connect to the Lerian API**

```bash
curl https://api.lerian.studio/health
lerian config                 # which profile and file are in use
lerian auth login
```

**401 Unauthorized** — the API key or tenant ID in the active profile is wrong. Run `lerian config` to see which profile is active, then `lerian auth login` again with the right values.

**Ledger creation times out**

```bash
lerian midaz ledger describe <ledger-id>
lerian midaz ledger events <ledger-id>
```

**`lerian infra` fails before it starts** — run `lerian infra check`. It reports every missing dependency in one pass and makes no AWS call.

## 🧪 Testing & operations

- [`docs/testing-strategy.md`](./docs/testing-strategy.md) — test approach and layout.
- [`docs/ci-cd/README.md`](./docs/ci-cd/README.md) — the three workflows, the `tier-1` channel and the change gates.

```bash
make test          # all tests with -race and coverage
make test-unit     # fast tests only
make test-race     # race detector
make lint          # golangci-lint
```

Pull requests to `develop`, `release-candidate` and `main` run PR validation (lint, tests, coverage gate, title scope check). Pushes to those branches cut the release.

## 🛠️ Development

Requirements: Go 1.26+, Make. `kubectl` is needed only for the Midaz operations commands.

```bash
git clone https://github.com/LerianStudio/lerian-cli.git
cd lerian-cli
make build          # → build/bin/lerian
make test
```

### Trying a change without touching the release

`make dev` builds the working tree as `lerian-dev`, next to the installed release instead of over it:

```bash
make dev

lerian version        # whatever was installed from the releases page
lerian-dev version    # whatever is checked out right now

make dev-uninstall    # when you are done
```

Both live in `~/.local/bin`. The dev version string carries the branch and a `-dirty` suffix when the tree has uncommitted work. `make install` also exists, but it uses `go install` and creates a second binary called `lerian` in `$GOPATH/bin`; prefer `make dev`.

### Commits and pull requests

Commits follow [Conventional Commits](https://www.conventionalcommits.org/), and the type decides the release version. PR titles are checked against an allowed scope list (`auth`, `cli`, `cmd`, `config`, `deps`, `docs`, `infra`, `ledger`, `output`, `tests`, and others in [`pr-validation.yml`](./.github/workflows/pr-validation.yml)). See [`CONTRIBUTING.md`](./CONTRIBUTING.md).

## 📚 References

- Command references: [`docs/infra.md`](./docs/infra.md), [`docs/midaz.md`](./docs/midaz.md), [`docs/interactive-session.md`](./docs/interactive-session.md)
- CI/CD: [`docs/ci-cd/README.md`](./docs/ci-cd/README.md)
- Terraform templates: [lerian-terraform-foundation](https://github.com/LerianStudio/lerian-terraform-foundation)
- Release history: [`CHANGELOG.md`](./CHANGELOG.md)
- Contributing: [`CONTRIBUTING.md`](./CONTRIBUTING.md) · Security: [`SECURITY.md`](./SECURITY.md) (security@lerian.studio) · [`CODE_OF_CONDUCT.md`](./CODE_OF_CONDUCT.md)
- Issues: [GitHub Issues](https://github.com/LerianStudio/lerian-cli/issues)

## License

Apache License 2.0 — Lerian Studio. See [LICENSE](./LICENSE).

---

<div align="center">
  <sub>Built and maintained by <a href="https://github.com/LerianStudio">Lerian Studio</a></sub>
</div>
