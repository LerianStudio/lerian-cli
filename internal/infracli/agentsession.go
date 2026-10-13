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

	options := make([]option, 0, len(resolved))
	for _, candidate := range resolved {
		row := option{value: candidate.Profile.Name, label: candidate.Profile.Name}
		switch {
		case candidate.Err != nil:
			row.disabled = true
			row.note = "(unavailable)  " + shortReason(candidate.Err)
		default:
			row.note = "account " + candidate.Caller.Account
			if candidate.Profile.Region != "" {
				row.note += "  ·  " + candidate.Profile.Region
			}
		}
		options = append(options, row)
	}
	if len(options) == 0 {
		return "", "", "", fmt.Errorf("no AWS profile resolves. `aws configure` or `aws sso login` first")
	}

	chosen, err := ask.pick("Which AWS account?", "The cluster the agent runs in lives there.",
		"", options, "--profile")
	if err != nil {
		return "", "", "", err
	}

	for _, candidate := range resolved {
		if candidate.Profile.Name == chosen {
			return chosen, candidate.Caller.Account, candidate.Profile.Region, nil
		}
	}
	return chosen, "", "", nil
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

// shortReason trims a resolution failure to something that fits beside a row.
func shortReason(err error) string {
	text := err.Error()
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		text = text[:index]
	}
	const width = 60
	if len(text) > width {
		text = text[:width] + "…"
	}
	return text
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
