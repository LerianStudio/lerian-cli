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

# Copy what you configured into a repository of your own
lerian config repo ~/infrastructure
```

Run `lerian infra --help` for the full reference: every flag, the account guard,
the ordering rules, and the environment variables it reads.


## Choosing what to run

### Which environment an account is

The account and the environment are two questions, in that order:

```
  Which environment is this account?
  It picks the sizing the templates ship, and the state backend to use.
❯ dev   configured here  ·  state backend ready
  stg   production's shape, smaller  ·  not set up here yet
  prd   another account holds it: 905418424496
```

The environment used to be derived from the account — the first section whose
`account_id` matched. That made an account permanently one environment: a
sandbox configured as `dev` could never also be `stg` in the same checkout,
because the lookup found `dev` and stopped. Worse, on a fresh checkout it was
the first *free* slot, so a production account set up first got `dev`, which in
these templates is `db.t4g.micro`, single-AZ, one day of backups and no deletion
protection.

Each row says where that environment stands **for this account**, because that
decides what happens next:

| row says | what follows |
|---|---|
| `configured here · state backend ready` | straight on to what to operate on |
| `configured here · no state backend yet` | `bootstrap` is the only thing that can run |
| `not set up here yet` | `init` runs first, then the backend is settled |
| `another account holds it: …` | not selectable — one account per environment |

The answer settles three things at once, which is why it comes before `init`:
which `envs/<env>.tfvars` is written (the sizing), which `backend/<env>.hcl`, and
the name of the state bucket — `lerian-tfstate-<env>-<account>`. One answer
produces all three, so they cannot disagree.

And `bootstrap` stops being offered once there is a backend:

```
  [ ] bootstrap  already there — lerian-tfstate-dev-524121347244
```

It is the one row with nothing to do at that point, and leaving it selectable
invites a run whose entire output is "no changes". The question somebody
actually has — is my state backend set up? — is answered by the row saying so.
(`--target bootstrap` still works from the command line, for changing the
bucket's own settings.)

### The targets are asked once

Choosing an account that is not set up yet runs `init`, and `init` asks what to
configure. The run then used to ask what to operate on — a second list of thirty
rows whose only sensible answer was the one given seconds earlier.

It is now said rather than asked:

```
==> Target
  infra-base  — what you just configured
```

Read back from the disk, not from memory: `init` decides what to write by
asking, and the files it leaves are the only record both this run and a later
one can agree on. An account that was already set up still gets the question,
because nothing was decided a moment ago.

`r` still reaches the account question from the action one. A step that decides
without asking is invisible to it — going back to a screen that is not there
would return immediately and move forward again, so the key would appear to do
nothing.

### After a run

A run used to end by returning to the top menu, which threw away every answer
that produced it. After a plan the next thing anybody wants is one of two things
— to read what it would change, or to apply it — and both meant walking the
account, backend, target and action questions again to arrive back where they
already were.

```
  What now?
  Same targets, same account. apply runs them for real, after one confirmation.
❯ show what the plan would change  resource by resource, from the plan just made
  apply                            writes, after one confirmation
  output                           reads terraform output
  destroy                          removes what these targets created, after one confirmation
  back to the menu                 leaves this account and target
```

`destroy` sits last among the actions and never beside `apply`. Everything above
it is something done repeatedly; it is the one that cannot be undone, and a list
where the two are one keypress apart is a list that will eventually be
mis-pressed. The run itself still inverts the order — EKS before the VPC — and
still skips `bootstrap`, whose bucket holds the state of everything else.

The detail is read back from the saved plan, not from a second one taken a
minute later — it describes the exact plan an `apply` from this menu would run.
Destructive actions come first within each stack, because "2 to destroy" is the
line worth finding in forty; a replacement is reported as `replace` rather than
as a delete and a create, since for a database that is the difference between a
deploy and an outage.

The offer only exists where the plans do: they are deleted when the run returns,
so this question lives inside it.


## The cluster a run makes

### Pointing kubectl at the cluster

After an `apply` that produced a cluster, the post-run menu offers it:

```
  What now?
❯ point kubectl at the cluster   runs aws eks update-kubeconfig
  ...
```

The cluster's name comes from the EKS stack's own outputs, not from rebuilding
the templates' naming convention here — that would be a second implementation of
somebody else's rule, correct right up until they rename something.

It is offered whenever there is a cluster, and the menu asks again before every
draw rather than once before the first. The apply that creates a cluster is
chosen *from* this menu, so an answer taken beforehand is an answer about the
world as it was — and that is exactly the run where somebody needs the offer.

The `aws eks update-kubeconfig` command is printed only where nobody can be
offered the alternative: without a terminal, in CI. With one, telling somebody
to type what the next screen is about to do for them is noise.

When the row is missing, the reason is on screen. A run with no `eks` root
simply has no cluster and says nothing; a run that has one whose outputs cannot
be read says why — a twenty-three-minute apply can outlive the credential that
started it, and an offer that quietly is not there reads as a tool that forgot
rather than one that could not.

It checks the kubeconfig first and asks before overwriting:

```
==> kubectl
  cluster   example-dev-eks
  file      /Users/you/.kube/config
  an entry for this cluster is already there, pointing elsewhere:
    now   https://OLD.gr7.us-east-2.eks.amazonaws.com
    after https://NEW.gr7.us-east-2.eks.amazonaws.com

  Replace it? [type yes to continue · ctrl-c cancels]:
```

That case is worth the question because it reads two ways. A cluster destroyed
and recreated keeps its name and gets a new endpoint — the entry is stale and
replacing it is the fix, and the symptom is a `kubectl` that fails with `no such
host` rather than with a permission error. But an entry pointing elsewhere can
also be a different cluster of the same name in another account, and overwriting
that one silently moves `kubectl` off something somebody is working against.

Pointing at the endpoint already there asks nothing: re-running
`update-kubeconfig` is how a broken context gets repaired, and confirming a
no-op teaches people to say yes without reading. A kubeconfig that cannot be
parsed is an error rather than "nothing will be overwritten" — that sentence
would be a guess about a file whose contents are unknown.

The file itself is written by the AWS CLI. Its format — contexts, users, the
exec credential plugin and the arguments that plugin wants from this version of
the CLI — is not this tool's to author. Reading it to see what is there is one
thing; writing it is another.

### Pointing kubectl at a cluster that already exists

```bash
lerian config kubeconfig
```

Asks which account, then which cluster, then runs `aws eks update-kubeconfig`.
The post-run menu covers the cluster an `apply` just made; this covers the rest
of the time — a cluster a colleague created, or one from a run long finished, in
an account this checkout may know nothing about.

The profiles are resolved before being offered, because a profile name says
nothing about which account it reaches, and the account is what is being chosen.
Clusters are listed per region — EKS is per region, and "the clusters in this
account" is not a question the API answers — starting with the profile's own
region, since that is where somebody's clusters usually are. If that region
holds none, it offers to look in another rather than stopping.

Overwriting an entry asks first, by the same rule as below — and only when it
would actually change something. The cluster's endpoint is read with one
`describe-cluster` so "the entry is already correct" and "the entry points at a
cluster that no longer exists" stop looking the same.

Then it checks the context it just wrote, with `kubectl get --raw /version`:

```
  kubectl now talks to example-dev-eks
  the cluster answered: v1.36.4-eks-cfb47f5
```

One call, no RBAC beyond being authenticated, and it fails differently for each
of the three things that actually go wrong — so the report says which:

| what comes back | what it means |
|---|---|
| `no such host` | the entry points at an endpoint that no longer exists; a cluster destroyed and recreated keeps its name and gets a new one |
| `Unauthorized` | the credential reached the cluster and was refused: this identity needs an EKS access entry |
| nothing, for ten seconds | the endpoint may be reachable only from allowed addresses — check `allowed_api_access_cidrs` against this machine |

That first row is the one worth having. It reads like broken DNS and is not, and
it is exactly what cost an afternoon here before this existed.


## Taking the estate with you

```bash
lerian config repo ~/work/my-infrastructure
```

The templates are another repository on another release cycle, and an estate
that lives inside a checkout of them has to ask permission to change anything.
This writes a copy that does not:

```
==> Exporting
  from      ~/.lerian/lerian-terraform-foundation  (the managed path)
  to        /Users/you/work/my-infrastructure
  roots     examples/aws/bootstrap, examples/aws/infra-base/vpc, examples/aws/infra-base/eks
  modules   naming
  config    environments.conf, dev.hcl, stg.hcl and 1 more

  45 file(s), one commit, branch main.

  Next:
    cd /Users/you/work/my-infrastructure
    git remote add origin <url>
    git push -u origin main
```

**It is also offered where the thought occurs.** The menu that follows a run
carries the same row, between `output` and `destroy`. The moment somebody has a
reason to want this is the moment they are looking at what was just built — not
later, having guessed that a command they have never seen exists.

**And once more on the way out.** After an apply, leaving that menu asks:

```
  Before you go
  Last offer; afterwards: lerian config repo <path>
❯ copy this into a repository of your own  the roots you configured, their mo…
  leave                                    nothing else is written
```

A row is only a row. "back to the menu" reads like the way out of a finished
job, and somebody who scrolled past the copy without knowing what it was for
loses the offer the moment the menu closes. Asked after an apply only — a plan
built nothing to take away — and only until the copy exists, because a prompt
that comes back after being answered is one people learn to dismiss. `r` goes
through it too; `q` and ctrl-c do not, since those mean stop now.

**What goes in.** The roots that have an `envs/<env>.tfvars` — what was
configured, not all twenty-seven products, which would be directories of someone
else's decisions. The modules those roots reach are **followed**, through each
other, by reading the `source` lines: a list kept in Go would be a second copy of
what the HCL says and would go stale the first time a root picks up a dependency.
Registry and git sources are left alone — they are fetched, not copied.

**A directory that already has something in it is asked about, not refused.**
"Give a path that does not exist yet" is the right answer when the occupant is
somebody's work and a pointless obstacle when it is last week's export of the
same estate — which is the common case:

```
  /Users/you/infrastructure already has something in it
  12 file(s), a git repository, 3 commit(s), and commits no remote has

  Replace it?
  What is there is not on any remote. Replacing it loses it for good.
❯ leave that directory alone   nothing is removed
  replace everything there     deletes what is in that directory, then writes the export
```

What is there is described first, because nobody can decide from the words "not
empty": whether that directory is a scratch copy or six months of work is the
entire question, and git knows the answer — how many files, whether it is a
repository, whether anything in it was never committed, whether any commit is
missing from every remote.

**Work that exists nowhere else is asked about twice**, the second time by typing
`yes`. One keypress is not the right price for a directory no clone anywhere
holds. A repository whose commits are all pushed gets the one question, and the
line above says why: that copy survives, this directory does not.

**Some paths are refused whatever the answer** — your home directory, the root of
a filesystem, the templates checkout this run is reading (or any directory
holding it), and any directory holding the one the command is running in. A
confirmation is consent to lose what was described, and in those the two are not
the same thing.

That list is the checkouts something actually depends on — the one resolved for
this run, the one recorded in the config, the managed paths — and **not**
"anything shaped like a checkout". Recognizing them by shape was wrong in the one
direction that matters: an export taken before the layout changed has an
`examples/aws/_modules` and an `examples/aws/backend` of its own, so last week's
export of the estate was protected as though it were the templates, and the offer
to replace it could never appear.

Declining re-asks for the path when the question came from the post-run menu:
"leave that directory alone" means "somewhere else", not "never mind". Outside a
terminal nothing is asked and nothing is removed — the refusal stands, now saying
what is in the way.

**What stays behind.** `.terraform` (a download cache: 1.5 GB against 6.5 MB of
content), state, saved plans, and the `*.tfvars-example` files. The example is
the question and the `.tfvars` beside it is the answer; shipping both invites
editing the one Terraform does not read.

**The `.gitignore` is not the templates'.** That one ignores `envs/*.tfvars`,
`backend/*.hcl` and `environments.conf`, which is right in a repository of
templates and exactly backwards here — in this repository those files are the
content. What stays ignored is what no repository should carry.

**It stops before the remote, unless you say otherwise.** Where your
infrastructure gets published is not this tool's guess to make — so it asks,
which is a different thing from guessing:

```
  This repository is ready. Publish it?
  It is already a git repository with one commit, here on this machine.
❯ keep it here                  nothing leaves this machine
  create it on GitHub and push  gh repo create, then push this commit
```

**Local is where the cursor starts.** The repository on disk is finished and is a
complete answer on its own; the other row publishes an estate's layout to a
server, and a stray enter must not be what does that.

**Where it goes is asked, not assumed.** `gh` creates in the personal account
when the name is unqualified, and somebody whose work lives in an organization
finds that out after the push — with the estate on a server under their own name,
and a second repository to delete:

```
  Where should it be created?
  The account signed in to gh, and the organizations it belongs to.
❯ octocat             your account
  acme                organization
  acme-labs           organization
  somewhere else      an owner this login's token cannot list
```

The name is then qualified with it — `acme/estate`, never a bare `estate` that
`gh` would place somewhere else. Skipped entirely for an account with no
organizations, which would be a question with one answer. The list comes from the
API and needs the `read:org` scope; without it only the account is listed, and
the line above says so rather than letting a short list read as "those
organizations do not exist".

Publishing takes four separate answers, because each is something somebody could
want different: yes, this owner, this name, this visibility. **Private is the default and the
row the cursor starts on.** The repository holds no credentials, but it is a map
of an estate — account numbers, VPC layout, cluster names — and public is a
decision to arrive at on purpose.

**Public takes a typed answer, and the warning names what it would publish:**

```
  A public repository is readable by anyone, including crawlers.
  Pushing this publishes:
    AWS account 524121347244
    state bucket lerian-tfstate-dev-…, lerian-tfstate-prd-…, lerian-tfstate-stg-…
    and the layout of the estate: subnets, cluster names, sizing
  Making it private later does not unpublish what was already read.

  Publish octocat/infrastructure publicly? [type yes to continue · ctrl-c cancels]:
```

The identifiers are read out of the export itself — `environments.conf` and
`backend/*.hcl` — rather than described in the abstract. "It may contain
sensitive information" states a possibility and gets clicked past; the account
number is a fact, and seeing it is the difference between a warning and a
decision. Declining returns to the visibility question rather than ending the
export, because somebody who just declined public almost always wants the other
row. The description is written rather than asked
for: it is the same sentence every time — what this is, which `lerian-cli` made
it, and from which templates tag — and a prompt whose answer is always the same
answer should have been a default.

**What goes wrong is turned into what to do.** `gh` exits 1 for every failure, so
the difference between them is entirely in the text:

| what gh said | what the CLI does |
|---|---|
| `Name already exists` | asks for another name, in place — everything else is ready and the fix is one word |
| `failed to push`, `Permission denied (publickey)` | says the repository **was** created and the push is what failed, and does **not** offer to add a remote that is already there |
| `HTTP 401`, `Bad credentials` | `gh auth login` |
| `HTTP 403`, `Resource not accessible`, a missing scope | `gh auth refresh -s repo` |
| `no such host`, `dial tcp` | GitHub is unreachable; the export is on disk either way |
| `HTTP 404` on an owner | that organization does not exist or is not one you can create in |

Anything unrecognized passes through unchanged rather than acquiring a guess —
`gh`'s own message is usually the better one, and a wrong hint costs more than no
hint. Every failure but the taken name stops; there is nothing this screen can do
about a missing scope, and asking again would produce the same failure twice.

**A taken name is two different situations**, so the repository that is there is
read before anything is offered:

```
  octocat/infrastructure already exists: private, last pushed to 2026-10-01T10:00:00Z

  What should happen to octocat/infrastructure?
  This commit is the whole history of the export; pushing it over replaces what is there.
❯ use a different name   leaves that repository alone
  push over it           force push — the history there is replaced by this one
```

It might be last week's export of this same estate, or six months of somebody
else's work. Only the operator knows which, and "that name is taken" is not
something to decide from — so GitHub is asked what is at the name, and the answer
is printed.

Pushing over takes a typed answer on top of the row, because a force push
replaces a history on a server other people may have cloned. An **empty**
repository skips that second question: there is nothing to replace, and asking
twice about nothing is how confirmations stop being read. A repository `gh`
cannot describe is never offered for overwrite at all — only the other answer
remains.

The remote is written in the protocol this machine's `gh` authenticates with. An
`https` remote where the credentials are an ssh key asks for a password nobody
has.

It goes through `gh` rather than the GitHub API. `gh` already holds the
credential, in the system keyring, shared with every other `gh` on the machine; a
token this CLI stored itself would be a second place for a secret to live and a
second place to revoke it from. If `gh` is installed but logged out, it offers
`gh auth login` and hands over the terminal — the login prints a code to paste
into a browser, and a login whose output this tool buffered would be a prompt
nobody can see. Then it asks `gh` again rather than assuming, because
`gh auth login` exits zero on paths that leave no usable credential.

Declining, no `gh`, a failed create — every one of them falls back to the three
commands, said out loud:

```
  Next:
    cd /Users/you/work/my-infrastructure
    git remote add origin <url>
    git push -u origin main
```

**The roots land at the top level.** In the templates they live under
`examples/aws/`, which is the right name there and the wrong one in a repository
that is somebody's actual estate — the first thing a client reads in their own
infrastructure should not be a word saying it is a demonstration.

```
  bootstrap/           infra-base/vpc/       infra-base/eks/
  _modules/naming/     backend/dev.hcl       environments.conf
```

Nothing was rewritten to achieve that. The directory is dropped from every path
at once, which leaves each `source = "../../_modules/..."` pointing exactly where
it did: removing the same leading directories from both ends of a relative path
is a no-op. If anything ever sits outside `examples/aws/` — a module reached from
above it — the prefix is kept instead, because a repository that reads nicely and
does not `terraform init` is worse than one with an awkward directory name.

**It writes a Makefile, and that is the front door.**

```bash
make plan eks dev          # one root
make plan infra-base dev   # every root under it, in order
make plan all dev          # every root there is
make roots                 # what can be run, and in what order
make check                 # fmt and validate every root
make help
```

Running one root by hand is five arguments, two of which cannot be guessed: the
backend file for the environment, and the state key for that root. People get
those right the first time by copying them out of the README and wrong every
time after.

**The roots are found, not listed.** A root is a directory with an `envs/` in it,
and its state key is its own path — both hold for every root the templates ship.
So a service added next month, in the same shape, appears in `make help`, gets
its own short name and its own `<name>-plan` shortcut, with no change to the
file. A generated list would have been accurate on the day of the export and
wrong the first time somebody added something, which is the point of handing the
repository over at all.

A short name two roots would answer to — `products/midaz/postgres` and
`products/ledger/postgres` are both "postgres" — is left out rather than given to
one of them, and no shortcut is generated for it. Both stay reachable by their
full path.

**A directory holding roots runs all of them.** `make plan infra-base dev` is the
whole of `infra-base`, and `all` is everything. The **order** is the other thing
discovery cannot work out — nothing in a directory says the network comes before
the cluster in it — so the order the CLI applied them in is written into the
file, and anything found later runs after it. `destroy` goes the other way: a
cluster cannot outlive the network it sits in.

```
  destroy these in dev, in order:
    infra-base/eks
    infra-base/vpc
  Type yes to continue:
```

A group that writes **confirms once**, for all of it, and lists what it would
touch. Asking per root turns one decision into three, and three prompts in a row
is a thing people answer without reading.

`apply` and `destroy` ask before they write, the same bar the CLI sets. The
bootstrap is named rather than found, because nothing in a directory says "this
one creates the backend": it gets a workspace and no `-backend-config`, which is
the other thing discovery cannot work out on its own.

**Every Terraform file says where it came from.**

```hcl
# Generated by lerian-cli v1.4.0 (Lerian Studio)
# Source: lerian-terraform-foundation v1.11.0
#
# This is a copy, not a link. Edit it freely — nothing reaches back.
```

In the files, not only in the README: the README is read once, and these are read
every time somebody opens the estate. Six months on, "where did this root come
from, and can I pull a newer one" is asked with a `.tf` file on the screen. It
goes into `.tf`, `.tfvars`, `.hcl` and `environments.conf` — everything Terraform
reads, and nothing else. A copy that rewrote arbitrary files would be a copy
nobody can trust.

**The README it writes is written for whoever opens it months later**, without
the person who ran the export in the room. It says what this is — the
infrastructure the Lerian applications run on — that it was generated **once, as
a bootstrap**, and that `lerian-cli` does not maintain it and will not update it.
From there it is ordinary Terraform belonging to whoever owns the repository.

It also gives the exact commands to plan **each** root, rather than one worked
example. The `key=` of a root's state is derived from its directory and cannot be
guessed, and a wrong one does not fail — Terraform initializes an empty state and
plans to create an estate that already exists. The keys come from `Unit.StateKey`,
the same call the runner makes, so the two cannot drift.

Three traps get their own paragraphs, because each one was hit while verifying
this:

- **The bootstrap's state did not travel**, because state never belongs in a
  repository. A plan there starts from nothing and offers to create the backend
  again — nine resources that already existed, in the run that found this.
- **A plan proposing to create everything** means the state is empty, not that the
  estate is missing. The README gives the two commands that tell those apart.
- **A later root planned before an earlier one is applied** fails reading its data
  sources (`no matching EC2 VPC found`), which reads as a broken configuration and
  is not.

**The first commit is authored by `lerian-studio <noreply@lerian.studio>`**, not
by whoever ran the export. It is a generated tree; attributing it to the person
at the terminal makes `git log` read as though they wrote four thousand lines of
Terraform. Everything after it is theirs.

### Signing in to GitHub

```bash
lerian config github
```

Who `gh` is signed in as on this machine, and how to change it:

```
  ==> GitHub
  active     octocat  used by gh repo create
  also       octocat-bot
  protocol   https  how the remote of a created repository is written

  GitHub
  Changes what gh does, for every tool on this machine.
❯ sign in to another account   gh auth login — an organization's host, or a second account
  switch the active account    decides where gh repo create puts a repository
  set how remotes are written  ssh or https — gh config set git_protocol
  sign out                     removes the credential from the keyring, for every tool
  back                         changes nothing
```

`lerian config repo` already signs you in on the way past, but that is a question
asked in the middle of doing something else, and it cannot answer the ones that
come up afterwards: signed in as the wrong account, needing a second one for an
organization, taking the credential off a machine being handed on.

**Which account is active is read, not assumed from the order.** It decides where
`gh repo create` puts a repository, and `gh` marks it on a line of its own —
`Active account: true`. It happens to print that one first today, and "today" is
not a guarantee worth resting a repository's owner on.

**The menu fits what is there.** No switch with one account, which would be a
question with one answer; no sign-out with no login behind it. Signing out is
confirmed, because the credential is in the system keyring and every `gh` on the
machine reads it — including the one in your other terminal.

The protocol row is the one `gh` setting worth surfacing: it decides whether the
remote of the exported repository is `git@github.com:…` or `https://github.com/…`,
and therefore whether pushing to it asks for a password every time. The one in
force is named in its note rather than disabled — re-picking it is a harmless
no-op, and a greyed-out row reads as "you may not have this".

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
