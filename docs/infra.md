# Infrastructure (`lerian infra`)

Reference for the `lerian infra` and `lerian config` command groups. For the
short version, see the [README](../README.md#usage).

## Infrastructure commands

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
  ok       templates    ~/.lerian/lerian-terraform-foundation @ v1.6.0
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
    [ ] flowker      needs the state backend — run bootstrap first
```

**Every run after.** The backend is there, and the whole catalog is on the
table — with the rows saying which of them this checkout has variables for:

```
  What do you want to operate on?
  ❯ [x] infra-base   the VPC then the cluster
    [ ] flowker      documentdb valkey                    ·  not configured here yet
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
lerian infra --env dev --target <product> --action helm-values --format yaml
```

Run `lerian infra --help` for the full reference: every flag, the account guard,
the ordering rules, and the environment variables it reads.

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
  checkout   ~/.lerian/lerian-terraform-foundation
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

### Knowing whether bootstrap has run

`backend/<env>.hcl` is written by `bootstrap`, never committed, and its absence
is how a checkout looks when nobody has bootstrapped. But that is a fact about
the **checkout**, not about the account — a fresh clone, or a colleague who ran
bootstrap from their own machine, produces a missing file over a bucket that has
existed for months. Acting on the file alone sends somebody to create a second
state backend beside the one their infrastructure's state is already in.

So when the file is missing, the CLI asks the account:

```
==> State backend
  no examples/aws/backend/dev.hcl here — asking the account

  A state backend already exists in this account. Use it?
  Adopting writes examples/aws/backend/dev.hcl from what is in the account.
❯ lerian-tfstate-dev-524121347244  this environment's own backend · us-east-2
  lerian-tfstate-stg-524121347244  made for stg · us-east-2
  create a new one                 bootstrap makes lerian-tfstate-dev-524121347244
```

That is `aws s3api list-buckets` filtered by the account suffix, plus
`get-bucket-location` and a `describe-table` for the lock table — fact, read off
the account, not inferred. Adopting writes the four lines `bootstrap` would have
written, which is what the old error told you to do by hand.

Every state bucket the account holds is offered, not only the one whose name
matches this environment — including the ones made for other environments, and
ones whose suffix is not an environment this tool knows (`lerian-tfstate-sandbox-…`
shows as *named by hand*). The search is bounded: a bucket has to carry the
`lerian-tfstate-` prefix the templates give it and end with this account's id, so
an account's unrelated buckets never appear. The note carries what decides
whether adopting is safe — the region (adopting one in the wrong region fails at
`terraform init` with a redirect that reads like anything but a region problem)
and whether a lock table exists (without one, concurrent runs are unprotected).

It never overwrites an existing `backend/<env>.hcl`: that file is the bucket the
state is under, and replacing it points a stack at state it has never seen. An
empty account says so plainly and `bootstrap` is the answer. A lookup that fails
— no permission, no network — is reported and the run continues: unknown is not
no, and blocking a run over a question that only prevents a duplicate bucket
would trade a small risk for a certain stoppage. With no terminal, nothing is
looked up at all; adopting is a decision, and CI would be paying for an API call
to print something nobody asked for.

### Getting a checkout in the first place

With none on the machine, `infra` offers to fetch one instead of asking where
something that is not there is:

```
  There is no templates checkout on this machine. Get one?
  The Terraform templates every stack is rendered from. About 30 MB, cloned with git.
❯ Clone it into ~/.lerian/lerian-terraform-foundation  this tool's own directory
  Clone it somewhere else                              you choose the directory
  I already have a clone                               give the path to it
```

Then it asks which release, from the tags that exist and that this binary can
read, newest first — rather than making you go and look one up for a flag.

Every prompt names both ways out. In a menu, `r` goes back one question and `q`
leaves; where you type an answer, the line reads `q cancel · ctrl-c quit`; at a
confirmation, `[type yes to continue · ctrl-c cancels]`.
ctrl-c works at any of them — the line editor runs in raw mode, where the
keypress is delivered to the CLI rather than as a signal, and it is read as
"stop", not as a broken read. Leaving prints `canceled.`, never an error; the
exit status is still non-zero, so `lerian infra apply && deploy` cannot mistake
a confirmation nobody gave for a successful apply.

The confirmations deserve their own note, because ctrl-c there used to do
nothing at all. The run installs a signal handler — it exists so an interrupt
stops `terraform` cleanly instead of orphaning a state lock — and that handler
consumes SIGINT and cancels the run's context. A blocking read does not notice a
canceled context, so the prompt simply sat there, at the one question standing
in front of writing files. The read now watches the context, so the key works.

The default lives under `~/.lerian`, beside the configuration, so everything the
CLI manages on a machine is in one directory. It is a perfectly ordinary git
checkout: open it, read it, run `terraform` in it by hand. **Clone it somewhere
else** takes any directory you like and records it, so later runs find it
without a flag; `--templates-dir` does the same for a single run.

A clone made before the move, at `~/lerian/lerian-terraform-foundation`, is
still found. That location is read and never written to: a checkout already on a
machine — with `environments.conf` and `tfvars` inside it that were never
committed — must not be silently abandoned for an empty directory next door.

### Pointing at a clone you already have

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
| the managed path `~/.lerian/lerian-terraform-foundation` | where `init --clone` puts one |
| the old managed path `~/lerian/lerian-terraform-foundation` | read, never written — a clone made before the move |

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
whether anything in it was never committed. Uncommitted work and commits that
were never pushed are the parts a fresh clone cannot bring back; the count
covers the first, so check `git log` against the remote before deleting a
checkout you have worked in. The cursor starts on the row that deletes nothing.

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
