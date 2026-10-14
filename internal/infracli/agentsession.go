package infracli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// AgentTarget is the cluster an agent command acts on, and how it was reached.
//
// Carried rather than assumed. Every agent command used to act on whatever the
// current kubeconfig context happened to be, which meant the answer to "which
// cluster did that install go into?" was "whichever one you last looked at" —
// and nothing on screen said which that was.
type AgentTarget struct {
	Profile string
	Region  string
	Account string
	Cluster string
	// Context is the kubeconfig context, passed to helm so the destination does
	// not depend on what is current when it runs.
	Context string
}

// Described renders the target for the line above an operation.
func (t AgentTarget) Described() string {
	if t.Cluster == "" {
		return "the current kubeconfig context"
	}
	return fmt.Sprintf("%s · account %s · %s", t.Cluster, t.Account, t.Region)
}

// PrepareAgent walks everything an agent command needs before it can act.
//
// The same shape as a run of `infra`, for the same reason: the tools, then the
// account, then what to operate on. An agent command reaches a real cluster and
// installs a workload into it; arriving there through three questions somebody
// answered on purpose is worth more than arriving silently through a kubeconfig
// file nobody looked at.
func PrepareAgent(ctx context.Context, out io.Writer, clusters infra.ClusterLister) (AgentTarget, error) {
	ask := newPrompter(out)
	if err := agentChecks(ctx, out); err != nil {
		return AgentTarget{}, err
	}

	profile, account, region, err := askAgentAccount(ctx, ask, out)
	if err != nil {
		return AgentTarget{}, err
	}

	cluster, err := askAgentCluster(ctx, ask, out, clusters, profile, region)
	if err != nil {
		return AgentTarget{}, err
	}

	target := AgentTarget{Profile: profile, Region: region, Account: account, Cluster: cluster}

	// Pointed, every time, rather than trusted. A kubeconfig entry for this
	// cluster may exist and point at a cluster of the same name in another
	// account, or carry a token that expired days ago.
	var mu sync.Mutex
	spin := newSpinner(&mu, out, "pointing kubectl at "+cluster)
	err = infra.UpdateKubeconfig(ctx, cluster, region, profile)
	spin.Stop()
	if err != nil {
		return AgentTarget{}, fmt.Errorf("cannot point kubectl at %s: %w", cluster, err)
	}
	target.Context = infra.ClusterARN("aws", region, account, cluster)

	fmt.Fprintf(out, "\n%s\n", newStyle(out).bold("==> Target"))
	fmt.Fprintf(out, "  cluster   %s\n", cluster)
	fmt.Fprintf(out, "  account   %s  (%s)\n", account, profile)
	fmt.Fprintf(out, "  region    %s\n", region)
	return target, nil
}

// agentChecks is the preflight: the tools an agent command actually shells out
// to, and nothing else.
//
// terraform is not among them. The agent is a Helm release and this command
// never runs Terraform, so demanding it would refuse a working machine over a
// tool this path does not use.
func agentChecks(ctx context.Context, out io.Writer) error {
	results := []checkResult{
		checkAWSCLI(ctx),
		checkHelm(),
	}
	return reportChecks(out, results, "")
}

// askAgentAccount picks the AWS account the cluster lives in.
func askAgentAccount(ctx context.Context, ask *prompter, out io.Writer) (profile, account, region string, err error) {
	var mu sync.Mutex
	spin := newSpinner(&mu, out, "resolving aws profiles")
	_, resolved := checkAWSSession(ctx, infra.CLIIdentity{})
	spin.Stop()

	options, targets := agentAccountOptions(resolved)
	if len(options) == 0 {
		return "", "", "", fmt.Errorf("no AWS profile was found. `aws configure` creates one")
	}

	chosen, err := ask.pick("Which AWS account?", "The cluster the agent runs in lives there.",
		"", options, "--profile")
	if err != nil {
		return "", "", "", err
	}

	for _, candidate := range resolved {
		if candidate.Profile.Name != chosen {
			continue
		}
		if candidate.Usable() {
			return chosen, candidate.Caller.Account, candidate.Profile.Region, nil
		}

		// Choosing a profile whose session has expired is choosing to log in. The
		// alternative is a menu where every row is unavailable and the only way
		// forward is a command this tool could have run.
		signedIn, err := offerLogin(ctx, ask, out, targets[chosen])
		if err != nil {
			return "", "", "", err
		}
		if !signedIn {
			return "", "", "", fmt.Errorf("not signed in to %s, so there is no account to read clusters from",
				chosen)
		}
		return resolveAfterLogin(ctx, out, chosen)
	}
	return chosen, "", "", nil
}

// agentAccountOptions builds the account rows, and the login each unusable one
// would need.
//
// Three states, the same three the infra menu distinguishes, because the right
// thing to say differs: a session that can be revived is worth choosing, an
// access key that does not work is worth choosing to see why, and a [default]
// section with nothing but a region is worth neither.
func agentAccountOptions(resolved []infra.ResolvedProfile) ([]option, map[string][]infra.SSOTarget) {
	options := make([]option, 0, len(resolved))
	targets := make(map[string][]infra.SSOTarget, len(resolved))

	for _, candidate := range resolved {
		name := candidate.Profile.Name
		if name == "" {
			name = "credentials in this environment"
		}
		row := option{value: candidate.Profile.Name, label: name}

		switch {
		case candidate.Usable():
			row.note = "account " + candidate.Caller.Account
			if candidate.Profile.Region != "" {
				row.note += "  ·  " + candidate.Profile.Region
			}
		case candidate.Profile.CanSignIn:
			// Not disabled: this is the row somebody needs when every session has
			// expired, which is most mornings.
			row.note = "session expired — choose to log in"
			targets[candidate.Profile.Name] = []infra.SSOTarget{{
				Session: candidate.Profile.SSOSession,
				Profile: candidate.Profile.Name,
			}}
		case candidate.Profile.Source == "credentials":
			row.note = "the access key does not work — choose it to see why"
		default:
			// Nothing to revive and nothing to fix.
			row.disabled = true
			row.note = "no credentials configured — aws configure --profile " + candidate.Profile.Name
		}
		options = append(options, row)
	}
	return options, targets
}

// resolveAfterLogin asks AWS who the profile is now that it is signed in.
func resolveAfterLogin(ctx context.Context, out io.Writer, profile string) (string, string, string, error) {
	var mu sync.Mutex
	spin := newSpinner(&mu, out, "checking "+profile)
	_, resolved := checkAWSSession(ctx, infra.CLIIdentity{})
	spin.Stop()

	for _, candidate := range resolved {
		if candidate.Profile.Name == profile && candidate.Usable() {
			return profile, candidate.Caller.Account, candidate.Profile.Region, nil
		}
	}
	return "", "", "", fmt.Errorf("%s still does not resolve after signing in", profile)
}

// askAgentCluster lists what is actually in the account and lets one be picked.
//
// Listed rather than typed: a cluster name is long, easy to mistype, and a typo
// produces "cluster not found" from a tool three layers down.
func askAgentCluster(
	ctx context.Context,
	ask *prompter,
	out io.Writer,
	clusters infra.ClusterLister,
	profile, region string,
) (string, error) {
	if region == "" {
		answered, err := ask.ask("Which region?", "Where the cluster is.", "us-east-1", "--region")
		if err != nil {
			return "", err
		}
		region = strings.TrimSpace(answered)
	}

	var mu sync.Mutex
	spin := newSpinner(&mu, out, "listing clusters in "+region)
	names, listErr := clusters.ListClusters(ctx, profile, region)
	spin.Stop()
	if listErr != nil {
		return "", fmt.Errorf("cannot list clusters in %s: %w", region, listErr)
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no EKS cluster in %s for profile %s.\n"+
			"`lerian infra --target infra-base --action apply` creates one, or pick another region",
			region, profile)
	}

	options := make([]option, 0, len(names))
	for _, name := range names {
		options = append(options, option{value: name, label: name})
	}
	return ask.pick("Which cluster?", "The agent is installed into it.", "", options, "--kube-context")
}

// Interactive reports whether questions can be asked at all, for a caller
// outside this package deciding whether to run the preflight.
func Interactive() bool {
	return newPrompter(nil).interactive
}

// carried is the target a deploy already established, for the offer that
// follows it. Set for the length of one session and read once.
var carried *AgentTarget

// CarryAgentTargetARN records the cluster a run just built, so the agent
// install offered afterwards does not ask which cluster it is.
//
// The ARN is taken rather than assembled: it is what the EKS stack reported,
// and it is the kubeconfig context verbatim. Assembling one from account and
// region guesses the partition, and guesses wrong in GovCloud and in China.
func CarryAgentTargetARN(profile, region, account, cluster, arn string) {
	if arn == "" {
		arn = infra.ClusterARN("aws", region, account, cluster)
	}
	carried = &AgentTarget{
		Profile: profile,
		Region:  region,
		Account: account,
		Cluster: cluster,
		Context: arn,
	}
}

// CarriedAgentTarget returns what a deploy established, if anything.
func CarriedAgentTarget() (AgentTarget, bool) {
	if carried == nil {
		return AgentTarget{}, false
	}
	return *carried, true
}

// Spin starts the same spinner the infra run uses, for a caller outside this
// package. The agent's operations are the same shape — seconds of waiting on
// something remote with nothing to show — and they should not look different.
func Spin(mu *sync.Mutex, out io.Writer, label string) *spinner {
	return newSpinner(mu, out, label)
}

// Heading renders a section heading in the style the rest of the CLI uses.
func Heading(out io.Writer, text string) string {
	return newStyle(out).bold(text)
}
