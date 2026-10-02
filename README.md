# Lerian CLI

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![GitHub Release](https://img.shields.io/badge/release-v1.0.0--beta-green.svg)](https://github.com/lerian-studio/lerian-cli/releases)

Official command-line interface for the Lerian platform. Manage your infrastructure products and deployments.

## Supported Products

### Midaz (Available Now)

Midaz is a ledger system for managing assets, operations, and multi-tenancy environments.

**Features:**
- **Ledger Management** - Create, list, describe, and delete ledger deployments
- **Multi-Region Support** - Deploy to SaaS regions or private infrastructure
- **Multiple Deployment Modes** - SaaS, private, and sandbox environments
- **Operations Tools** - Logs, port-forwarding, SQL execution, backups, and events
- **Kubernetes Integration** - Direct access to deployed resources

### Infrastructure (Available Now)

Drive the Terraform roots of
[lerian-terraform-foundation](https://github.com/LerianStudio/lerian-terraform-foundation)
on AWS, from bootstrap to the per-product services.

**Features:**
- **Environment Bootstrap** - State bucket and lock table, then the VPC and the EKS cluster
- **Target Resolution** - Products and services discovered from the checkout; combine with commas
- **Account Guard** - Three checks before anything runs, with no flag to bypass them
- **Dry Run** - Resolve and print the whole execution plan without a single AWS call
- **Helm Values** - Read `helm_values` back out of the applied state

Ported from `lerian-infra-cli`, which this CLI replaces.

### Future Products

- **Flowker** - Coming soon
- **Reporter** - Coming soon
- **Tracer** - Coming soon
- **Fees** - Coming soon

## Core Features

- **Authentication** - Profile-based authentication with API key support
- **Multiple Output Formats** - Table, JSON, and YAML output options
- **Multi-Product Support** - Unified CLI for all Lerian products

## Installation

### Quick Install (Recommended)

The easiest way to install the Lerian CLI is using the install script:

```bash
curl -fsSL https://raw.githubusercontent.com/LerianStudio/lerian-cli/main/scripts/install.sh | sh
```

**Prerequisites:** [GitHub CLI (gh)](https://cli.github.com/) installed and authenticated with access to the repository.

#### Install Options

```bash
# Install latest version
curl -fsSL https://raw.githubusercontent.com/LerianStudio/lerian-cli/main/scripts/install.sh | sh

# Install specific version
curl -fsSL https://raw.githubusercontent.com/LerianStudio/lerian-cli/main/scripts/install.sh | sh -s -- --version v1.0.0

# Custom install directory
INSTALL_DIR=/usr/local/bin curl -fsSL https://raw.githubusercontent.com/LerianStudio/lerian-cli/main/scripts/install.sh | sh
```

### Other Installation Methods

<details>
<summary><strong>Manual Download</strong></summary>

Download the latest release for your platform from the [releases page](https://github.com/LerianStudio/lerian-cli/releases).

```bash
# Example for Linux amd64
gh release download --repo LerianStudio/lerian-cli --pattern "lerian_*_Linux_x86_64.tar.gz"
tar -xzf lerian_*_Linux_x86_64.tar.gz
sudo mv lerian /usr/local/bin/
```

</details>

<details>
<summary><strong>From Source</strong></summary>

```bash
git clone https://github.com/LerianStudio/lerian-cli.git
cd lerian-cli
make install
```

</details>

<details>
<summary><strong>Using Go Install</strong></summary>

```bash
go install github.com/LerianStudio/lerian-cli/cmd/lerian@latest
```

> Note: Requires Go 1.25+ and access to the private repository via GOPRIVATE.

</details>

<details>
<summary><strong>Linux Packages (.deb/.rpm)</strong></summary>

Download the appropriate package from the [releases page](https://github.com/LerianStudio/lerian-cli/releases):

```bash
# Debian/Ubuntu
gh release download --repo LerianStudio/lerian-cli --pattern "*.deb"
sudo dpkg -i lerian_*.deb

# RHEL/Fedora
gh release download --repo LerianStudio/lerian-cli --pattern "*.rpm"
sudo rpm -i lerian_*.rpm
```

</details>

### Verify Installation

```bash
lerian --version
```

### Shell Completions

Enable auto-completion for your shell:

```bash
# Bash
lerian completion bash > /etc/bash_completion.d/lerian
# Or for current user only:
lerian completion bash >> ~/.bashrc

# Zsh
lerian completion zsh > "${fpath[1]}/_lerian"
# Or add to ~/.zshrc:
echo 'source <(lerian completion zsh)' >> ~/.zshrc

# Fish
lerian completion fish > ~/.config/fish/completions/lerian.fish

# PowerShell
lerian completion powershell > lerian.ps1
```

## Quick Start

### The interactive session

`lerian` with nothing after it opens a session: it offers the commands, runs the
one you pick, and asks again when that command is done. `q` closes it.

```
  ██╗     ███████╗██████╗ ██╗ █████╗ ███╗   ██╗        ██████╗██╗     ██╗
  ██║     ██╔════╝██╔══██╗██║██╔══██╗████╗  ██║       ██╔════╝██║     ██║
  ██║     █████╗  ██████╔╝██║███████║██╔██╗ ██║ █████╗██║     ██║     ██║
  ██║     ██╔══╝  ██╔══██╗██║██╔══██║██║╚██╗██║ ╚════╝██║     ██║     ██║
  ███████╗███████╗██║  ██║██║██║  ██║██║ ╚████║       ╚██████╗███████╗██║
  ╚══════╝╚══════╝╚═╝  ╚═╝╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝        ╚═════╝╚══════╝╚═╝

  ─────────────────────────────────────────────────────────────  v1.7.0

  What do you want to do?
  ↑↓ move · enter choose · r back · q cancel
  ❯ auth     Authentication commands
    infra    Deploy the AWS stacks of lerian-terraform-foundation
    version  Print version information
```

Choosing a command that only groups others — `auth` — offers its subcommands
rather than printing a help page:

```
  Which auth command?
  ❯ login   Configure authentication credentials
    logout  Remove authentication credentials
```

The menu is a shorter list than the command set. `midaz` is reached with ledger
ids, regions and sizes a menu has no way to ask for, so picking it from a list
would land you on a help page rather than on anything you chose to do — it stays
a command (`lerian midaz ledger list` is unaffected) and stays out of the menu.

`r` goes back one question; `q` leaves. The questions come in a sequence, and a
wrong turn on the first one used to cost the whole run — the only way to correct
it was ctrl-c, which throws away the answers that were right along with the one
that was not. On the first question, and on the main menu, there is nothing
before: `r` there backs out of the run, and out of nothing, respectively.

The session exists because these commands come in sequences — check the machine,
then init; init fails, read what it says, run it again — and each of those used
to cost a fresh start. A command that fails does not close it either. The exit
code is the last command's, so `lerian && something` still means what it says.

The wordmark lights up a line at a time and the rule draws itself across, which
takes about a fifth of a second and happens once per session. Below the width the
wordmark needs it gives way to a single line — a wrapped wordmark is six broken
lines, and the terminal decides where it wraps.

Only on a terminal. Piped, redirected or in CI, `lerian` prints its help exactly
as before: a prompt there waits for an answer that is never coming. The banner
follows the same rule, and `LERIAN_NO_BANNER=1` turns it off for anyone who has
seen it enough times. `NO_COLOR` and `TERM=dumb` are honored throughout — both
skip the animation and print the banner whole.

### First Steps

1. **Login to Lerian Platform**
   ```bash
   lerian auth login \
     --api-url https://api.lerian.studio \
     --api-key YOUR_API_KEY \
     --tenant-id YOUR_TENANT_ID
   ```

2. **Create Your First Midaz Ledger**
   ```bash
   lerian midaz ledger create \
     --name my-first-ledger \
     --region us-east-1 \
     --env dev
   ```

3. **List Your Midaz Ledgers**
   ```bash
   lerian midaz ledger list
   ```

4. **Get Midaz Ledger Details**
   ```bash
   lerian midaz ledger describe <ledger-id>
   ```

## Usage

### Global Flags

- `--config` - Config file path (default: `$HOME/.lerian/config.yaml`)
- `--profile, -p` - Profile to use (default: `default`)
- `--output, -o` - Output format: `table`, `json`, `yaml` (default: `table`)
- `--help, -h` - Show help for any command
- `--version, -v` - Show version information

### Authentication

```bash
# Login to platform
lerian auth login \
  --api-url https://api.lerian.studio \
  --api-key your-api-key \
  --tenant-id your-tenant-id

# Login with custom profile
lerian auth login \
  --profile production \
  --api-url https://api.lerian.studio \
  --api-key prod-key \
  --tenant-id prod-tenant

# Logout from current profile
lerian auth logout
```

### Infrastructure Commands

All infrastructure commands start with `lerian infra`. Unlike the rest of the CLI,
this group takes flags rather than subcommands — it kept the command line of the
`lerian-infra` binary it replaces, so anything written against that binary keeps
working with `lerian infra` in front of it.

Picking `infra` from the session — or running `lerian infra` with no `--env` —
checks the machine before it asks anything:

```
  ==> Environment check
  ok       terraform    /opt/homebrew/bin/terraform
  ok       aws          /opt/homebrew/bin/aws
  ok       templates    ~/lerian/lerian-terraform-foundation @ v1.6.0
  ok       aws session  8 of 9 profiles resolve: dev, stg, prd and 5 more

  4 checks, all ok.
```

It then asks which account, every run:

```
  Which AWS account?
  Everything is created there. The state backend and the sizing follow from it.
  ❯ lerian-sandbox           account 524121347244  ·  us-east-2
    default                  session expired — choose to log in
    other-profile            account 239025757440  ·  not set up here yet — choosing it sets it up
    sign in as someone else  ends the session for every AWS tool on this machine
```

The region is named beside the account, because an account is a place and so is a
region: naming one without the other describes half of where the resources land.
The environment is not named at all — `dev` picks `backend/dev.hcl` and
`envs/dev.tfvars`, which matters to this tool and to nobody choosing where to
deploy. It appears only where two rows reach the same account in the same region,
and the name is the one thing telling them apart.
It is on the confirmation before an apply for the same reason — an apply into the
right account and the wrong region does not fail, it creates a second copy of
everything somewhere nobody is looking.

Setting up a new account asks for the region rather than taking it from the
profile. The profile's own region is offered as the suggestion; where every
resource is created is not something to inherit from a profile configured for
something else.

Three kinds of row, and every one of them is choosable:

- **ready** — deploys into that account;
- **expired** — choosing it logs into that profile's session, then carries on;
- **not set up** — choosing it sets the account up, here, without sending you to
  a second command or asking you to pick an environment name.

The last row signs out and back in, which is the only way to arrive as a
different identity. It is a row rather than something the command does at
start-up, because `aws sso logout` clears a token in `~/.aws/sso/cache` that
**every** AWS client on the machine reads — another terminal, Terraform, anything
on the shared config.

### Typing only where there is nothing to choose from

Anything with a known set of answers is a list. The region is one:

```
  Which AWS region will the infrastructure be created in?
  Every resource lands here. Moving later means recreating them.
  ❯ us-east-1       N. Virginia
    us-east-2       Ohio
    sa-east-1       São Paulo
    …
    another region  type a code this list does not have — AWS adds regions, this copy ages
```

The place is named beside the code, because `sa-east-1` is not where most people
know São Paulo to be. The last row is the way past the list, and it exists because
the list is a copy that ages — AWS adds regions and this file does not hear about
it. What is typed there is checked for shape, not for membership, so a region
newer than this copy is accepted and a typo is not.

The same applies to the Kubernetes API address once it has been detected: use what
was found, or give another. Typing it back character by character is the work the
detection just did.

What stays typed is what has no set to offer: the path to the templates checkout —
which is editable, with Tab completing directories — and the egress address when
detection fails outright.

### The first run on an account, and every one after

There are two shapes to a run, and what separates them is whether the state
backend exists — `backend/<env>.hcl`, which records the S3 bucket and lock table
the state lives in. It is written by `bootstrap`, not by `init`.

**First time on an account.** Nothing exists: no section in `environments.conf`,
no backend, no bucket. Choosing the account sets up the first, and then
`bootstrap` is the only target offered — everything else needs somewhere to keep
its state, and `bootstrap` is what creates it:

```
  What do you want to operate on?
  ❯ [x] bootstrap    state bucket and lock table
    [ ] infra-base   needs the state backend — run bootstrap first
    [ ] midaz        needs the state backend — run bootstrap first
```

**Every run after.** The backend is there, and the whole catalog is on the
table — with the rows saying which of them this checkout has variables for:

```
  What do you want to operate on?
  ❯ [x] infra-base   the VPC then the cluster
    [ ] midaz        documentdb postgres rabbitmq valkey  ·  not configured here yet
    [ ] fetcher      documentdb rabbitmq s3 valkey        ·  not configured here yet
```

`init` writes `envs/<env>.tfvars` for the targets it is given, which is usually
`infra-base`. The catalog lists every product, so most rows have no variables
until somebody asks for them — and choosing one used to spend two more answers
before failing with `4 of 4 stacks are NOT READY`, a true message arriving three
steps late.

The list used to offer everything either way, so a first run could spend two
answers on a stack that fails at `terraform init` reporting a bucket that does not
exist.

### A machine with nothing configured

This CLI is not only run on machines that already have our profiles. On one with
no `~/.aws` at all — the normal state of a machine somebody has just been handed —
the check offers to set it up rather than sending you away:

```
  This machine has no AWS credentials. Set them up now?
  Either one writes to ~/.aws, which is where every AWS tool reads them from.
  ❯ sign in to an SSO portal  aws configure sso — what an organization hands out
    use an access key         aws configure — an access key id and secret
    not now                   leaves the instructions below
```

Credentials already in the environment count as a session, with no `~/.aws`
needed: CI exports them, and so does anyone who has a key rather than a portal.
They appear in the account list as "credentials in this environment", and map to
whichever environment declares `profile = -`.

### The three-account ceiling

An account has to occupy one of `backend/<env>.hcl` and `envs/<env>.tfvars`, and
the templates provide three sets: `dev`, `stg`, `prd`. So a checkout holds at most
three accounts, and the CLI picks the free slot itself — which one is bookkeeping,
not a decision worth a question. A fourth account says so plainly rather than
failing later with a missing file; the answer is a second checkout.

`dev`, `stg` and `prd` never appear as a question. They stay in the model because
they name the files above, and the account guard still checks the account before
anything runs — but what is decided here is the account, and the environment
follows from it.

Every profile in `~/.aws` gets a row, ordered by what can be done with it. One
reaching a configured account is ready. One whose session has expired is
choosable — choosing it is how you say "log me into that one", and the login is
for that profile's session. One reaching an account with no section cannot be
deployed into, because there is no state backend and no variables file for it, so
the row says how to create one instead of offering a choice that fails two
questions later.

The block appears whether or not anything is wrong: which checkout and which
terraform a run is about to use is worth a line each, and showing them only on
failure means never seeing them on the run that matters. A scripted invocation —
one that named its `--env` and asked nothing — keeps the output it always had.

When something is missing, the remediation follows the table, and a missing
session is offered a way out:

```
  missing  aws session  not logged in

  Log in to AWS now?
  Runs aws sso login --sso-session <your-session>, which opens a browser.
  ↑↓ move · enter choose · q cancel
  ❯ log in now  opens the browser and waits
    cancel      leaves the instructions below
```

The verdict leads each row so that scanning for what failed is running an eye
down the left edge rather than a ragged right one.

When there is no session, it offers to log in rather than telling you to leave.
The AWS CLI is already a verified dependency and the session name is already in
the profile it just read, so sending you to another program and asking you to
start over buys nothing. It runs `aws sso login`, which opens the browser, and
then re-checks. Declining leaves the report and the command to run by hand.

One login per `[sso-session]`, not one per profile: profiles behind the same
session are revived together. A profile backed by a static key in
`~/.aws/credentials` has no session to revive, so none is offered for it.

It runs before the questions, not after them: the three questions — which
environment, which stacks, plan or apply — take real thought, and a machine that
cannot run anything makes all three answers worthless. And the credential is
checked the moment the environment names its profile, rather than when the first
stage tries to start, for the same reason: by then the target and the action have
been answered too.

`git` is not part of it. Only `init --clone` uses git, and a run that already has
its checkout never calls it.

```bash
# Verify this machine: dependencies, checkout, and what it would use
# (local only — makes no AWS call, so it works as a CI gate)
lerian infra check

# Write the configuration a fresh checkout needs
lerian infra init --env dev

# List the discoverable targets (no AWS call, no configuration read)
lerian infra --list

# Resolve and print the execution plan, touching nothing
lerian infra --env dev --target all --dry-run

# Stand up an environment, in order
lerian infra --env dev --target bootstrap        --action apply
lerian infra --env dev --target infra-base       --action apply
lerian infra --env dev --target shared-resources --action apply

# Read the helm values of a product back out
lerian infra --env dev --target midaz --action helm-values --format yaml
```

Run `lerian infra --help` for the full reference: every flag, the account guard,
the ordering rules, and the environment variables it reads.

### Midaz Product Commands

All Midaz commands start with `lerian midaz`.

#### Ledger Management

##### Create Ledger

**SaaS Deployment (Default):**
```bash
lerian midaz ledger create \
  --name my-ledger \
  --region us-east-1 \
  --env dev
```

**Private Deployment:**
```bash
lerian midaz ledger create \
  --name prod-ledger \
  --mode private \
  --region private-us-west-2 \
  --env prod \
  --size production \
  --agent-id <agent-uuid>
```

**Sandbox (7-day trial):**
```bash
lerian midaz ledger create \
  --name trial-ledger \
  --sandbox \
  --region us-east-1
```

**Available Options:**
- `--name` - Ledger name (required, 3-100 characters)
- `--region` - Deployment region (required)
- `--env` - Environment: `dev`, `staging`, `prod` (required unless `--sandbox`)
- `--mode` - Deployment mode: `saas`, `private` (default: `saas`)
- `--size` - Ledger size: `test`, `staging`, `production` (default: `test`)
- `--tps` - Transactions per second (10-10000)
- `--multi-az` - Enable multi-AZ deployment
- `--sandbox` - Create sandbox ledger with auto-expiration
- `--app-version` - Specific app version to deploy
- `--chart-version` - Specific Helm chart version
- `--agent-id` - Agent ID (required for private mode)

##### List Ledgers

```bash
# Table format (default)
lerian midaz ledger list

# JSON format
lerian midaz ledger list -o json

# YAML format
lerian midaz ledger list -o yaml

# Use specific profile
lerian midaz ledger list --profile production
```

##### Describe Ledger

```bash
lerian midaz ledger describe <ledger-id>
```

##### Delete Ledger

```bash
lerian midaz ledger delete <ledger-id>
```

#### Operations

##### View Logs

```bash
# Show logs
lerian midaz ledger logs <ledger-id>

# Follow logs in real-time
lerian midaz ledger logs <ledger-id> --follow

# Show last 100 lines
lerian midaz ledger logs <ledger-id> --tail 100
```

##### Port Forwarding

```bash
# Forward local port 8080 to ledger service port 8080
lerian midaz ledger port-forward <ledger-id> 8080:8080
```

##### Execute SQL

```bash
# Run SQL query
lerian midaz ledger exec <ledger-id> "SELECT COUNT(*) FROM accounts;"
```

##### Backup Database

```bash
# Create backup
lerian midaz ledger backup <ledger-id> --output ./backup.sql
```

##### View Kubernetes Events

```bash
# View events for troubleshooting
lerian midaz ledger events <ledger-id>
```

##### Check Available Versions

```bash
# List available app and chart versions
lerian midaz ledger versions
```

## What the CLI remembers, and how to forget it

```bash
lerian config          # what it has written down, and where
lerian config reset    # forget it, as if the CLI had never run here
```

There is one file — `~/.lerian/config.yaml` — and it holds two things: where the
templates checkout is, and the profiles `lerian auth login` creates. `reset`
removes that file, so the next run asks what it asked the first time.

```
  ==> This tool
  config     ~/.lerian/config.yaml
  profile    default   (Lerian platform, not AWS)
  logins     none — lerian auth login creates one

  ==> Templates
  checkout   ~/lerian/lerian-terraform-foundation
  found by   the managed path — found by convention, not recorded
  version    v1.11.0

  ==> AWS
  config     ~/.aws/config
  profiles   7: default, acme-dev, acme-production and 4 more
  sessions   acme-sso
  whether they work is an AWS call: lerian infra check makes it

  ==> Tools
  terraform  /opt/homebrew/bin/terraform
  aws        /opt/homebrew/bin/aws
  git        /usr/bin/git
```

Grouped by who owns each thing, because the same word means different things in
different groups: a profile under **This tool** is a Lerian platform login, a
profile under **AWS** is a credential in `~/.aws`. Side by side with no headings
they read as one kind of thing.

It reads files and makes no AWS call — it has to work on a machine with no
network, and nobody opening a "where are things" page wants a round trip per
profile. What that costs is knowing whether the credentials work, so the page
names the command that answers it.


**"profile" means two different things in this CLI**, and both have a flag:
`lerian --profile` is a Lerian platform login kept in the file above, while
`lerian infra --profile` is an AWS profile from `~/.aws`. Two flags with one name
is a surface inherited from the `lerian-infra` binary; every place the word
appears now says which one it means.

It takes that file and **nothing else**. `~/.aws` belongs to the AWS CLI and every
tool on this machine reads it; a templates checkout is a git clone you made,
possibly with work in it. Neither is this command's to delete, and a "reset" that
took them would be an expensive surprise. `lerian infra cleanup` is the one that
removes caches and run logs, and it says the same thing about AWS.

### Pointing it at a checkout

```bash
lerian config templates /path/to/lerian-terraform-foundation   # record it
lerian config templates --clear                                # forget it
```

Picked off the menu instead, with no path to give, it asks — offering the
checkouts this machine already has, plus a line to type one and, when there is
something recorded, a row to forget it.

There are five ways to say where the templates are, and this is the one that
sticks. In order of precedence:

| | |
|---|---|
| `--repo <path>` | this run only |
| `$LERIAN_TF_REPO` | this shell only |
| the working directory, or one above it | the checkout you are standing in |
| **recorded** — `config templates` | **every run, until cleared** |
| the managed path `~/lerian/lerian-terraform-foundation` | where `init --clone` puts one |

A recorded path beats the managed one: that is a decision, this is a directory
that happens to exist somewhere conventional. It loses to the working directory,
because the checkout you are inside is the one you mean.

`--clear` forgets the path and leaves the directory alone — it is a clone you
made, possibly with work in it.

### Forgetting everything

It asks before removing, unless `--yes`. Outside a terminal, with no `--yes`, it
refuses rather than guessing.

Then it offers to delete the templates checkouts themselves — **one question per
directory**, answered separately from the first one:

```
  /Users/you/lerian/lerian-terraform-foundation
  cloned by this tool · 1 file changed and not committed · 412 MB
  Everything in it goes, committed or not, and no later run can bring it back.

  Delete this templates checkout?
  Deletes the directory and everything in it. This cannot be undone.
  ❯ keep the directory  nothing is deleted
    delete it           the directory and all its contents
```

Separate because the two answers undo differently: forgetting a path is undone
by the next run asking again, and deleting a git clone is undone by nothing. The
line above the question is the part worth reading — whose directory it is, and
whether anything in it was never committed, which is the only thing a fresh
clone cannot bring back. The cursor starts on the row that deletes nothing.

Only the checkouts this tool would use are in scope: the recorded one and the
managed path. **Not the directory you are standing in** — `reset` is run from
wherever you happen to be, and deleting the repository you are sitting in
because you were sitting in it is not a reset. Before anything is removed it
refuses any path that is not a checkout, so a config holding a stale or mistyped
directory cannot turn this into a recursive delete of whatever lives there now.

`--yes` alone never deletes a directory: it has meant "forget the configuration"
on every machine that already runs it, and widening that silently would change
what those invocations do. `--delete-templates` is the flag that says it, and
the only way to reach the deletion with no terminal to ask at.

`~/.aws` is never touched. It belongs to the AWS CLI, and every tool on the
machine reads it.

From the menu, picking `config` offers what it can do:

```
  Which config command?
  ❯ show   Print the configuration and where it lives
    reset  Forget everything, as if the CLI had never run here
```

`reset` is last on purpose: the cursor starts on the first row, and a list that
opens on the command that removes things makes the most likely keypress the
destructive one.

## Configuration File

Configuration is stored at `~/.lerian/config.yaml`:

```yaml
current-profile: default
profiles:
  default:
    api-url: https://api.lerian.studio
    api-key: your-api-key-here
    tenant-id: your-tenant-uuid-here
  production:
    api-url: https://api.lerian.studio
    api-key: prod-api-key
    tenant-id: prod-tenant-uuid
```

### Using Profiles

```bash
# Use production profile
lerian --profile production ledger list

# Set profile during login
lerian auth login --profile production --api-url ... --api-key ... --tenant-id ...
```

### Custom Config File

```bash
lerian --config /path/to/config.yaml ledger list
```

## Midaz Deployment Modes

Midaz ledgers support three deployment modes:

### SaaS Mode (Default)
Multi-tenant deployment on Lerian-managed infrastructure.
- Shared Kubernetes clusters
- Multiple availability zones
- Managed by Lerian team
- Quick provisioning

### Private Mode
Single-tenant deployment on your own infrastructure.
- Your Kubernetes cluster
- Full control over resources
- Data stays in your network
- Requires Lerian Agent

### Sandbox Mode
Temporary ledger for testing and trials.
- Auto-expires after 7 days
- Limited to test size
- Dev environment only
- Quick setup for evaluation

## Regions

### SaaS Regions
- `us-east-1` - US East (N. Virginia)
- `us-west-2` - US West (Oregon)
- `eu-west-1` - Europe (Ireland)
- `ap-southeast-1` - Asia Pacific (Singapore)
- `sa-east-1` - South America (São Paulo)

### Private Regions
- Use `private-*` prefix for agent-connected regions
- View available private regions: `lerian agent list` (future)
- Requires Lerian Agent deployment

## Midaz Ledger Sizes

| Size | TPS | Resources | Use Case |
|------|-----|-----------|----------|
| `test` | 10 | Minimal | Development and testing |
| `staging` | 100 | Medium | Pre-production environments |
| `production` | 1000 | Full | Production workloads |

## Troubleshooting

### Connection Issues

**Problem:** Cannot connect to Control Plane API

**Solution:**
```bash
# Verify API endpoint
curl https://api.lerian.studio/health

# Check configuration
cat ~/.lerian/config.yaml

# Re-authenticate
lerian auth login
```

### Authentication Errors

**Problem:** API returns 401 Unauthorized

**Solution:**
```bash
# Verify API key and tenant ID
cat ~/.lerian/config.yaml

# Login with correct credentials
lerian auth login --api-url ... --api-key ... --tenant-id ...
```

### Deployment Timeout

**Problem:** Ledger creation times out

**Solution:**
```bash
# Check deployment status
lerian midaz ledger describe <ledger-id>

# View events for more details
lerian midaz ledger events <ledger-id>
```

## Testing

### Test Coverage

Current test coverage: **~86%** (83.7% for config package, 90.9% for version package)

Target coverage: **80%+** for all packages

### Running Tests

```bash
# Run all tests with coverage
make test

# Run unit tests only (fast)
make test-unit

# Run tests with coverage report
make test-coverage

# Run tests with race detector
make test-race

# Run tests in verbose mode
make test-verbose

# Run benchmarks
make test-bench
```

### Test Organization

Tests follow Go best practices:
- **Unit tests**: Fast, isolated tests for individual functions
- **Table-driven tests**: Parameterized test cases for comprehensive coverage
- **Integration tests**: Tests with real file I/O and system interactions
- **Benchmark tests**: Performance testing

### Coverage by Package

| Package | Coverage | Target | Status |
|---------|----------|--------|--------|
| `internal/version` | 90.9% | 90%+ | Complete |
| `internal/config` | 83.7% | 80%+ | Complete |
| `internal/output` | 0% | 80%+ | In Progress |
| `internal/kubectl` | 0% | 70%+ | Planned |
| `internal/client` | 0% | 70%+ | Planned |
| `cmd/auth` | 0% | 60%+ | Planned |
| `cmd/midaz/ledger` | 0% | 60%+ | Planned |

### Test Examples

**Unit Test Example** (`internal/config/config_test.go`):
```go
func TestLoad_ValidConfigFile(t *testing.T) {
    // Setup: Create temporary config
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, ".lerian", "config.yaml")

    // Test: Load and verify
    config, err := Load()

    if err != nil {
        t.Fatalf("Load() error = %v", err)
    }
}
```

**Table-Driven Test Example**:
```go
func TestGetProfile(t *testing.T) {
    tests := []struct {
        name        string
        config      *Config
        profileName string
        wantErr     bool
    }{
        {"valid profile", validConfig, "test", false},
        {"profile not found", emptyConfig, "missing", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            profile, err := tt.config.GetProfile(tt.profileName)
            // assertions...
        })
    }
}
```

### Continuous Integration

Tests run automatically on:
- Pull requests to `develop`, `release-candidate`, and `main`
- Push to `develop`, `release-candidate`, and `main`
- Scheduled weekly security scans

See `.github/workflows/` for CI/CD configuration.

## Development

### Prerequisites

- Go 1.26 or higher
- kubectl (for Kubernetes operations)
- Make

### Building from Source

```bash
# Clone repository
git clone https://github.com/lerian-studio/lerian-cli.git
cd lerian-cli

# Install dependencies
make deps

# Build binary
make build

# Run tests
make test
```

### Trying a change without touching the release

`make dev` builds the working tree as **`lerian-dev`**, next to the installed
release rather than over it:

```bash
make dev

lerian version        # whatever was installed from the releases page
lerian-dev version    # whatever is checked out right now

make dev-uninstall    # when you are done
```

The two live side by side in `~/.local/bin`, so testing a change never costs you
the binary you depend on, and never needs a release to be cut first. The dev
version string carries the branch and a `-dirty` suffix when the tree has
uncommitted work, so a binary built from a half-finished change says so when
asked.

`make install` still exists, but it uses `go install` and therefore creates a
**second binary also called `lerian`** in `$GOPATH/bin`. Which of the two runs
then depends on the order of your `PATH`. Prefer `make dev`.

### Project Structure

```
lerian-cli/
├── cmd/
│   ├── lerian/          # Main entry point
│   ├── auth/            # Authentication commands
│   └── midaz/           # Midaz commands
│       └── ledger/      # Ledger management
├── internal/
│   ├── client/          # HTTP API client
│   ├── config/          # Configuration management
│   ├── kubectl/         # Kubernetes operations
│   └── output/          # Output formatting
├── docs/                # Documentation
├── examples/            # Example files
└── scripts/             # Build and automation scripts
```

## Documentation

- [Installation Guide](docs/getting-started/installation.md) *(coming soon)*
- [Command Reference](docs/commands/) *(coming soon)*
- [Architecture Overview](docs/architecture/overview.md) *(coming soon)*
- [Contributing Guide](CONTRIBUTING.md)
- [Security Policy](SECURITY.md)

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details on:
- Code of conduct
- Development setup
- Coding standards
- Pull request process
- Commit conventions

## Support

- **Documentation:** https://docs.lerian.studio *(coming soon)*
- **Issues:** [GitHub Issues](https://github.com/lerian-studio/lerian-cli/issues)
- **Community:** https://community.lerian.studio *(coming soon)*
- **Email:** support@lerian.studio

## License

Copyright © 2025 Lerian Studio. All rights reserved.

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.

## Acknowledgments

Built with:
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [YAML v3](https://github.com/go-yaml/yaml) - YAML support
- [UUID](https://github.com/google/uuid) - UUID generation

## Version History

See [CHANGELOG.md](CHANGELOG.md) for release history and changes.

---

**Current Version:** v0.1.0

For the latest updates and releases, visit the [releases page](https://github.com/lerian-studio/lerian-cli/releases).
