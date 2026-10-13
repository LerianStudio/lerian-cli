# The Lerian agent (`lerian agent`)

The agent is a process that runs inside your cluster, makes **outbound only**
connections to the Lerian control plane, and executes the Helm operations the
control plane gives it — install, upgrade, rollback, uninstall, diff, preflight,
status. One per cluster. Nothing reaches into the cluster from outside.

It does **not** provision cloud infrastructure. The VPC, the cluster and the
datastores belong to `lerian infra`; work items asking the agent for
infrastructure are refused.

```
release name    lerian-agent          (fixed by the chart)
namespace       lerian-system         (--namespace)
identity Secret lerian-agent-identity (kept on uninstall)
chart           oci://ghcr.io/lerianstudio/agent-helm
requires        Kubernetes >= 1.33, helm on PATH
```

## Two ways in, one installation

```bash
# Explicit, for a cluster that already exists
lerian agent install --control-plane https://cp.example.com --token lerian_enroll_...
```

Or from the menu at the end of a cluster deploy, where the answer to "which
cluster?" is the one just built:

```
What now?
  ❯ install the Lerian agent  connects this cluster to the Lerian control plane
```

Both end up in the same code. The second only asks what the CLI cannot work out
for itself.

Where the control plane is, it usually can: `lerian auth login` records the
platform's `api-url` in a profile, and the agent connects to that same platform.
So `--control-plane` defaults to the current profile's URL, the question offers
it as the answer Enter accepts, and only a cluster pointed at a different
control plane has to type one. The chart has no default of its own — it refuses
to render without the URL — so this is a value somebody always has to have, not
one worth asking for twice.

The offer appears after an `apply` that left a cluster behind, never after a
`plan`, and never preselected: it reaches a real cluster and needs a credential
nobody has lying around.

## Why Helm and not Terraform

The agent is a workload. Helm already keeps the release state in the cluster, so
a Terraform root would mirror that state in a remote backend and keep the two in
step for no gain. Worse, a root that finds its cluster with
`data "aws_eks_cluster"` makes "install into a cluster this CLI never created"
the awkward path — and that is the normal path.

What is lost is worth saying: `terraform destroy` used to remove the agent along
with everything else. Now `lerian agent uninstall` is a separate step somebody
can forget.

## The credential

Two forms, and the chart refuses to render without one:

| | where it comes from | `--agent-id` |
|---|---|---|
| **enrollment** | `POST /api/tenants/:tenantId/agents/enrollments` | must be empty — the agent is told its identity when it redeems the token, and keeps it in `lerian-agent-identity` |
| **per-agent** | `POST /api/tenants/:tenantId/agents` | required, from the same response |

An enrollment token is single-use and short-lived, which is what makes it safe
to hand to a cluster that does not exist yet.

**Either form reaches the cluster as a Secret.** `--existing-secret` is the way
the credential never passes through this process at all: put it in a Secret by
another route and name it here.

The token is written to a file with mode `0600` and removed when helm is done —
never `--set`, which would put it in the shell history, in `ps` output while
helm runs, and in the log of any CI that echoes its commands.

## Managed namespaces

`--managed-namespaces` is where the agent may **install**, not where it runs.
Each entry gets a `Role` and a `RoleBinding`, so **each must already exist** —
Kubernetes has nowhere to put a Role otherwise. A missing one fails the install,
and the failure prints the `kubectl create namespace` for each name.

Everything else lives in `--namespace` (default `lerian-system`): the Deployment,
the ServiceAccount, the Secrets, the ConfigMap.

## Commands

```bash
# Install, or re-install with new values
lerian agent install --control-plane https://cp.example.com --token lerian_enroll_... \
  --managed-namespaces apps,data

# Without the credential ever passing through the CLI
lerian agent install --control-plane https://cp.example.com --existing-secret agent-creds

# What is installed, and what it is running
lerian agent status

# Move to another chart version, keeping every other value
lerian agent upgrade --chart-version 1.0.2

# Remove it, keeping the identity Secret
lerian agent uninstall --yes
```

`--kube-context` picks the cluster explicitly; without it the current kubeconfig
context is used, which is what somebody running this by hand expects.

## What is checked before helm runs

Five rules, enforced in the CLI so the message names the flag to change rather
than a line inside a chart template:

- a credential is present — `--token` or `--existing-secret`
- not both, because the chart treats the Secret as the switch and would write a
  token nothing reads
- `--agent-id` is empty with an enrollment token
- `--control-plane` is `https://`, unless `--allow-insecure-http` (an isolated
  dev cluster only — the agent sends its bearer token on every request)
- `--image-repository`, when set, sits under the registries the agent will
  accept for its own self-update. Leaving `--allowed-registries` empty does not
  mean "any registry": it means the chart keeps its default of
  `ghcr.io/lerianstudio`, and the agent would refuse to update itself weeks
  later, in somebody else's terminal

## After installing

```bash
kubectl -n lerian-system get pods -l app.kubernetes.io/name=agent
kubectl -n lerian-system logs deploy/lerian-agent | grep "sent initial heartbeat"
```

The control plane shows the agent connected once that heartbeat lands. To check
the credential without side effects the image takes `--self-check`; do **not**
call `/work/poll` by hand, which claims a work item.

A failed install leaves the pod in place on purpose — `--atomic` would roll back
and delete the evidence. `lerian agent status` says where to read it.
