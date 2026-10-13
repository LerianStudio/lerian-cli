// Package agent installs and manages the Lerian agent in a Kubernetes cluster.
//
// The agent is a workload, not infrastructure: it runs inside the customer's
// cluster, connects outward to the Lerian control plane, and executes the Helm
// operations the control plane gives it. This package drives Helm directly
// rather than going through Terraform, because Helm already keeps the release
// state in the cluster and a second copy of that state in a remote backend
// bought nothing — while making "install into a cluster this CLI never created"
// the awkward path instead of the normal one.
package agent

import (
	"fmt"
	"strings"
)

// Fixed by the chart rather than chosen here.
const (
	// ReleaseName is what the chart installs as. Changing it would orphan the
	// release somebody else's upgrade is looking for.
	ReleaseName = "lerian-agent"
	// Chart is the OCI reference the chart is published at.
	Chart = "oci://ghcr.io/lerianstudio/agent-helm"
	// DefaultNamespace is where the agent runs, which is not where it installs.
	DefaultNamespace = "lerian-system"
	// IdentitySecretName is the name of the Secret holding the identity, not the
	// identity itself. The Secret survives an uninstall, so a reinstall keeps the
	// identity the control plane already knows.
	// #nosec G101 -- a Kubernetes object name, not a credential. The identity it
	// refers to is written by the agent into the cluster and never passes
	// through here.
	IdentitySecretName = "lerian-agent-identity"
	// EnrollmentPrefix marks a single-use token that carries no identity yet.
	EnrollmentPrefix = "lerian_enroll_"
	// ChartDefaultRegistries is what the chart allows when nothing is set.
	// Written down because a default living only in somebody else's values.yaml
	// cannot be reasoned about here.
	ChartDefaultRegistries = "ghcr.io/lerianstudio"
)

// Options is everything the chart needs that this CLI does not already know.
type Options struct {
	// Namespace is where the agent itself runs.
	Namespace string
	// ChartVersion pins the chart. Empty takes the newest published version,
	// which is what a cluster being set up today should get.
	ChartVersion string

	// ControlPlaneURL is the base URL the agent polls.
	ControlPlaneURL string
	// AllowInsecureHTTP permits a cleartext ControlPlaneURL. The agent sends its
	// bearer token on every request, so this is for an isolated dev cluster only.
	AllowInsecureHTTP bool

	// Token authenticates the agent. Either an enrollment token, single-use and
	// carrying no identity, or a per-agent token, which needs AgentID with it.
	Token string
	// AgentID accompanies a per-agent token and must be absent for an enrollment
	// one: the agent is told its identity when it redeems the enrollment.
	AgentID string
	// ExistingSecret names a Secret already in the cluster holding AGENT_TOKEN
	// (and AGENT_ID). It is the way the credential never passes through here.
	ExistingSecret string

	// ManagedNamespaces are the namespaces the agent may install into. Each gets
	// a Role and RoleBinding, so each must already exist.
	ManagedNamespaces []string

	// ImageRepository overrides where the agent image is pulled from.
	ImageRepository string
	// ImageDigest pins the image by digest.
	ImageDigest string
	// AllowedRegistries is what the agent will accept for its own self-update.
	AllowedRegistries string
	// ImagePullSecrets name Secrets for a registry that refuses anonymous reads.
	ImagePullSecrets []string

	// NoWait returns as soon as the objects are created, instead of waiting for
	// the agent to become ready.
	//
	// Readiness means /health/ready, and the agent is not ready until it has
	// registered with the control plane — so against a control plane that does
	// not exist yet, waiting is waiting for something that cannot happen. The
	// install itself is still a real test: the chart renders, the release is
	// created, the RBAC lands in the managed namespaces, and the pod starts.
	//
	// Off by default, because everywhere else a green install should mean a
	// running agent rather than a set of objects.
	NoWait bool
}

// UsesExistingSecret reports whether the credential stays out of this process.
func (o Options) UsesExistingSecret() bool { return o.ExistingSecret != "" }

// effectiveRegistries is what the agent will actually enforce, which is not
// always what was asked for: leaving AllowedRegistries empty does not mean "any
// registry", it means the chart keeps its own default.
func (o Options) effectiveRegistries() string {
	if o.AllowedRegistries != "" {
		return o.AllowedRegistries
	}
	return ChartDefaultRegistries
}

// imageAllowed reports whether ImageRepository sits under one of the allowed
// entries.
//
// By prefix rather than by cutting the repository into a fixed number of
// segments. Counting segments has to decide what "the registry" is before it
// can compare anything, and every answer is wrong for some input:
// "docker.io/agent" has two segments and no namespace, "agent" has one and no
// host at all, and a rule that skips what it cannot parse accepts exactly the
// repositories it exists to reject.
//
// The trailing slash makes it a prefix of the path and not of the string:
// without it "ghcr.io/lerianstudio-staging/agent" passes a list that only
// allows "ghcr.io/lerianstudio". Empty entries are dropped, so a list written
// with a trailing comma does not contribute the prefix "/".
func (o Options) imageAllowed() bool {
	for _, entry := range strings.Split(o.effectiveRegistries(), ",") {
		entry = strings.TrimSuffix(strings.TrimSpace(entry), "/")
		if entry == "" {
			continue
		}
		if strings.HasPrefix(o.ImageRepository, entry+"/") {
			return true
		}
	}
	return false
}

// Validate refuses what the chart would refuse, and what it would accept and
// then regret.
//
// Caught here rather than in a template, because the message can name the flag
// to change. Three of these mirror the chart's own rules; the fourth is about
// something that goes wrong weeks later, in somebody else's terminal, when the
// agent refuses to update itself from a registry nobody told it about.
func (o Options) Validate() error {
	if !o.UsesExistingSecret() && o.Token == "" {
		return fmt.Errorf("no credential: pass --token, or --existing-secret for a Secret already in " +
			"the cluster. The agent cannot authenticate without one of them")
	}
	if o.UsesExistingSecret() && o.Token != "" {
		return fmt.Errorf("--token and --existing-secret do both: the chart treats the Secret as the " +
			"switch, so the token would be written somewhere nothing reads")
	}
	if strings.HasPrefix(o.Token, EnrollmentPrefix) && o.AgentID != "" {
		return fmt.Errorf("--agent-id must be empty with an enrollment token: the agent is given its " +
			"identity when it redeems one, and keeps it in the Secret " + IdentitySecretName)
	}
	// The other half of the same rule. The chart refuses this too, in its own
	// template, so the install fails either way — but it fails naming a line in
	// secrets.yaml, and the thing to change is a flag.
	if o.Token != "" && !strings.HasPrefix(o.Token, EnrollmentPrefix) && o.AgentID == "" {
		return fmt.Errorf("--agent-id is required with a per-agent token: it comes from the same " +
			"POST /api/tenants/:tenantId/agents response as the token. An enrollment token " +
			"(" + EnrollmentPrefix + "...) is the form that carries no id")
	}
	if !o.AllowInsecureHTTP && !strings.HasPrefix(o.ControlPlaneURL, "https://") {
		return fmt.Errorf("--control-plane must be https://, or pass --allow-insecure-http. The agent "+
			"sends its bearer token on every request; cleartext is for an isolated dev cluster only (got %q)",
			o.ControlPlaneURL)
	}
	if o.ImageRepository != "" && !o.imageAllowed() {
		return fmt.Errorf("the agent's allowed registries (%s) do not contain %q, so it would refuse "+
			"its own self-update. Set --allowed-registries alongside --image-repository",
			o.effectiveRegistries(), o.ImageRepository)
	}
	return nil
}
