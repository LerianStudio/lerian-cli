package infracli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"sync"

	"github.com/lerian-studio/lerian-cli/internal/agent"
	"github.com/lerian-studio/lerian-cli/internal/infra"
)

// manageAgentFromMenu is what the row after a deploy opens.
//
// The four operations rather than the install alone. Somebody who has just
// planned against a running cluster most often wants to know what is in it, and
// an offer that can only install answers a question they did not ask — and asks
// for a control plane URL to do it.
func manageAgentFromMenu(ctx context.Context, ask *prompter, out io.Writer, point func() error) error {
	// The kubeconfig first, for every operation and not only the install: all
	// four reach the cluster, and reading the wrong one is as wrong as writing
	// to it.
	if point != nil {
		if err := point(); err != nil {
			return fmt.Errorf("cannot point kubectl at the cluster: %w", err)
		}
	}

	helm, err := agent.NewHelm()
	if err != nil {
		return err
	}
	if carried, ok := CarriedAgentTarget(); ok {
		helm.Context = carried.Context
	}

	// What is there now, so the menu can say it rather than offering an upgrade
	// for something that is not installed.
	var mu sync.Mutex
	spin := newSpinner(&mu, out, "reading the release")
	release, installed, err := helm.Installed(ctx, agent.DefaultNamespace)
	spin.Stop()
	if err != nil {
		return err
	}

	chosen, err := ask.pick("What about the agent?", agentState(release, installed), "",
		agentActions(installed), "")
	if err != nil {
		return err
	}

	switch chosen {
	case "status":
		printAgentStatus(out, release, installed)
		return nil
	case "install":
		return installAgentFromMenu(ctx, ask, out, nil)
	case "upgrade":
		return upgradeAgentFromMenu(ctx, helm, out, release)
	case "uninstall":
		return uninstallAgentFromMenu(ctx, ask, helm, out, release)
	}
	return nil
}

// agentState is the line under the question: what is installed, if anything.
func agentState(release agent.Release, installed bool) string {
	if !installed {
		return "Nothing is installed in " + agent.DefaultNamespace + " yet."
	}
	return fmt.Sprintf("%s is installed, %s, revision %s.", release.Chart, release.Status, release.Revision)
}

// agentActions offers what can actually be done to what is there.
func agentActions(installed bool) []option {
	if !installed {
		return []option{
			{value: "install", label: "install it", note: "needs a control plane URL and a token"},
			{value: "status", label: "show what is there", note: "reads the release, changes nothing"},
		}
	}
	return []option{
		{value: "status", label: "show what is there", note: "reads the release, changes nothing"},
		{value: "upgrade", label: "move it to another chart version", note: "keeps every other value"},
		{value: "install", label: "reconfigure it", note: "new control plane, token or namespaces"},
		{value: "uninstall", label: "remove it", note: "keeps the identity Secret"},
	}
}

// printAgentStatus is the same report `lerian agent status` prints.
func printAgentStatus(out io.Writer, release agent.Release, installed bool) {
	if !installed {
		fmt.Fprintf(out, "\n  No agent in %s.\n\n", agent.DefaultNamespace)
		return
	}
	fmt.Fprintf(out, "\n  release      %s\n", release.Name)
	fmt.Fprintf(out, "  namespace    %s\n", release.Namespace)
	fmt.Fprintf(out, "  chart        %s\n", release.Chart)
	fmt.Fprintf(out, "  app version  %s\n", release.AppVersion)
	fmt.Fprintf(out, "  revision     %s\n", release.Revision)
	fmt.Fprintf(out, "  status       %s\n", release.Status)
	if release.Status != "deployed" {
		fmt.Fprintf(out, "\n  What the agent says:\n    kubectl -n %s logs deploy/%s\n",
			release.Namespace, agent.ReleaseName)
	}
	fmt.Fprintln(out)
}

// upgradeAgentFromMenu moves the release to the newest published chart.
//
// No version asked for: the menu's answer to "which version" is "the current
// one", and somebody who wants a specific chart has `lerian agent upgrade
// --chart-version` for it.
func upgradeAgentFromMenu(ctx context.Context, helm *agent.Helm, out io.Writer, release agent.Release) error {
	// Not waiting when it was not ready to begin with: a new chart does not make
	// an agent that never registered become ready, and waiting for it spends the
	// timeout to report the state it was already in.
	noWait := release.Status != "deployed"

	fmt.Fprintf(out, "\n%s\n", newStyle(out).bold("==> Upgrading the Lerian agent"))
	fmt.Fprintf(out, "  from  %s\n", release.Chart)
	fmt.Fprintf(out, "  to    the newest published chart\n\n")

	var mu sync.Mutex
	spin := newSpinner(&mu, out, "upgrading in "+release.Namespace)
	err := helm.Reuse(ctx, "", release.Namespace, agent.DefaultTimeout, noWait)
	spin.Stop()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\n  Upgraded. %s\n\n", newStyle(out).dim("lerian agent status"))
	return nil
}

// uninstallAgentFromMenu removes the release, after the same confirmation a
// destroy gets.
func uninstallAgentFromMenu(
	ctx context.Context,
	ask *prompter,
	helm *agent.Helm,
	out io.Writer,
	release agent.Release,
) error {
	what := fmt.Sprintf("remove %s (%s) from %s", release.Name, release.Chart, release.Namespace)
	fmt.Fprintf(out, "\n  %s\n  The control plane will stop hearing from it.\n", what)
	if err := ask.confirm(ctx, out, what); err != nil {
		return err
	}
	var mu sync.Mutex
	spin := newSpinner(&mu, out, "removing from "+release.Namespace)
	err := helm.Uninstall(ctx, release.Namespace)
	spin.Stop()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\n  Removed. %s was kept, so a reinstall is the same agent.\n\n",
		agent.IdentitySecretName)
	return nil
}

// installAgentFromMenu is the second way into the one installation.
//
// `lerian agent install` is the first, and it is the one somebody reaches for
// when the cluster already exists. This is the offer at the end of a deploy,
// where the cluster was just built and the answer to "which cluster?" is
// obvious — so the only things left to ask are the ones this CLI cannot know:
// where the control plane is, and the token it issued.
//
// Both end up in agent.Install. Nothing about the installation depends on which
// way somebody arrived.
func installAgentFromMenu(ctx context.Context, ask *prompter, out io.Writer, point func() error) error {
	// The kubeconfig first, always. The chart goes wherever the current context
	// points, and "the cluster the operator last selected" is the wrong
	// destination for a release named after this one.
	if point != nil {
		if err := point(); err != nil {
			return fmt.Errorf("cannot point kubectl at the cluster, so the agent would install "+
				"into whichever one is current: %w", err)
		}
	}

	helm, err := agent.NewHelm()
	if err != nil {
		return err
	}
	if carried, ok := CarriedAgentTarget(); ok {
		helm.Context = carried.Context
	}

	opts, err := askAgentOptions(ask)
	if err != nil {
		return err
	}

	// Said before it is done: an install reaches a real cluster, and what it is
	// about to do should be readable before it happens rather than reconstructed
	// from helm's output afterwards.
	theme := newStyle(out)
	fmt.Fprintf(out, "\n%s\n", theme.bold("==> Installing the Lerian agent"))
	fmt.Fprintf(out, "  namespace          %s\n", opts.Namespace)
	fmt.Fprintf(out, "  control plane      %s\n", opts.ControlPlaneURL)
	fmt.Fprintf(out, "  credential         %s\n", describeCredential(opts))
	fmt.Fprintf(out, "  managed namespaces %s\n", describeNamespaces(opts))
	fmt.Fprintf(out, "  chart              %s\n\n", describeChart(opts))

	// The spinner rather than helm's own stream: a chart's NOTES.txt is a page
	// of headings that buries the one line saying what happened, and this is
	// seconds of waiting on a remote registry with nothing to show — which is
	// what the spinner is for everywhere else in this CLI.
	var mu sync.Mutex
	spin := newSpinner(&mu, out, "installing into "+opts.Namespace)
	err = agent.Install(ctx, helm, opts, agent.DefaultTimeout)
	spin.Stop()
	if err != nil {
		return err
	}

	// Read back rather than assumed: the chart version, the app version and the
	// revision are what helm recorded, and an install that resolved "newest"
	// only knows which one that was afterwards.
	release, _, _ := helm.Installed(ctx, opts.Namespace)
	target, _ := CarriedAgentTarget()
	PrintInstalled(out, target, opts, release)
	return nil
}

// describeChart names the version that will be installed.
func describeChart(opts agent.Options) string {
	if opts.ChartVersion == "" {
		return agent.Chart + "  ·  newest published"
	}
	return agent.Chart + "  ·  " + opts.ChartVersion
}

// PrintInstalled is what replaces helm's NOTES.txt: where it landed, what is
// running there, and the one thing worth doing next.
//
// Where matters as much as what. An install reaches one cluster in one account,
// and a report that names neither leaves somebody to remember which — across
// seven AWS profiles, that is a thing to be told rather than recalled.
func PrintInstalled(out io.Writer, target AgentTarget, opts agent.Options, release agent.Release) {
	theme := newStyle(out)
	fmt.Fprintf(out, "\n%s\n", theme.bold("==> Installed"))

	rows := [][2]string{}
	if target.Cluster != "" {
		rows = append(rows,
			[2]string{"cluster", target.Cluster},
			[2]string{"account", target.Account + accountSuffix(target.Profile)},
			[2]string{"region", target.Region},
		)
	}
	rows = append(rows, [2]string{"namespace", opts.Namespace})
	if release.Chart != "" {
		rows = append(rows, [2]string{"chart", release.Chart})
	}
	if release.AppVersion != "" {
		rows = append(rows, [2]string{"agent", release.AppVersion})
	}
	if release.Revision != "" {
		rows = append(rows, [2]string{"revision", release.Revision + "  ·  " + release.Status})
	}
	rows = append(rows,
		[2]string{"control plane", opts.ControlPlaneURL},
		[2]string{"may install into", describeNamespaces(opts)},
	)

	width := 0
	for _, row := range rows {
		if len(row[0]) > width {
			width = len(row[0])
		}
	}
	for _, row := range rows {
		fmt.Fprintf(out, "  %-*s  %s\n", width, row[0], row[1])
	}

	fmt.Fprintln(out)
	if opts.IsPlaceholder() {
		// Saying "it appears in the control plane once it heartbeats" would
		// promise what this install cannot do: there is nowhere to register, and
		// the pod sits at 0/1 forever.
		fmt.Fprintf(out, "  It has nowhere to register, so it retries and stays unready. What it is\n")
		fmt.Fprintf(out, "  doing:\n")
		fmt.Fprintf(out, "    %s\n", theme.dim("kubectl -n "+opts.Namespace+" logs deploy/"+agent.ReleaseName))
		fmt.Fprintf(out, "\n  Re-running this once there is a control plane address is the same\n")
		fmt.Fprintf(out, "  operation, and keeps the identity it already has.\n\n")
		return
	}

	fmt.Fprintf(out, "  It appears in the control plane once its first heartbeat lands:\n")
	fmt.Fprintf(out, "    %s\n\n", theme.dim("lerian agent status"))
}

// accountSuffix names the profile beside the account, when there is one.
func accountSuffix(profile string) string {
	if profile == "" {
		return ""
	}
	return "  (" + profile + ")"
}

// AskAgentOptions asks for an install, for a caller outside this package.
//
// `lerian agent install` with no flags arrives here. The alternative was what
// it did at first: answer somebody who picked "install" off a menu with the
// usage text of a command they never typed, which leaves them exactly where
// they started.
//
// Outside a terminal, prompter.ask already refuses and names the flag to pass,
// which is the same contract every other question in this CLI has. A second
// check here said the same thing in different words and was unreachable in
// practice.
func AskAgentOptions(out io.Writer) (agent.Options, error) {
	return askAgentOptions(newPrompter(out))
}

// askAgentOptions asks only what this CLI cannot work out for itself.
func askAgentOptions(ask *prompter) (agent.Options, error) {
	out := ask.out
	opts := agent.Options{Namespace: agent.DefaultNamespace}

	// Not asked. The control plane belongs to Lerian, so it is a constant this
	// CLI carries — asking for it would be asking somebody to supply a value
	// they have no way of choosing. `--control-plane` remains for a cluster
	// pointed at a different one, which is a deliberate exception rather than
	// the normal case.
	opts.ControlPlaneURL = agent.DefaultControlPlane("")

	if opts.ControlPlaneURL == "" {
		// The state this CLI is used in today: the address has not been decided.
		// Refusing to install until it exists would refuse everything that does
		// not depend on it — the chart, the RBAC, the pod.
		//
		// Both values, because a token without somewhere to redeem it is no more
		// useful than a URL with nothing to present.
		opts.ControlPlaneURL = agent.PlaceholderControlPlane
		opts.Token = agent.PlaceholderToken
		opts.NoWait = true

		opts.ManagedNamespaces = agent.DefaultManagedNamespaces

		fmt.Fprintf(out, "\n  %s\n", newStyle(out).bold("Installing without a control plane"))
		fmt.Fprintf(out, "  There is no Lerian control plane address yet, so the agent will start and\n")
		fmt.Fprintf(out, "  retry against %s, which never resolves. That\n", agent.PlaceholderControlPlane)
		fmt.Fprintf(out, "  exercises the chart, the RBAC and the pod — not the registration.\n")
		fmt.Fprintf(out, "  Re-running this once the address exists is the same command.\n")
		return opts, opts.Validate()
	}

	token, err := ask.ask("Enrollment token?",
		"Single-use, from POST /api/tenants/:tenantId/agents/enrollments. "+
			"The agent is told its identity when it redeems one.",
		"", "--token")
	if err != nil {
		return opts, err
	}
	opts.Token = strings.TrimSpace(token)

	// Not asked either. Which namespaces the agent writes to follows from what
	// Lerian deploys, not from whoever runs the installer, and the prompt had a
	// right answer nobody outside this repository could know.
	opts.ManagedNamespaces = agent.DefaultManagedNamespaces

	// Asked, because waiting is right when the control plane is there and a trap
	// when it is not: readiness means registered, so against a control plane that
	// does not answer this waits the whole timeout and then fails with the agent
	// running the entire time. Nothing the CLI can see tells it which case this
	// is — only the person who knows whether that URL is live yet.
	if opts.IsPlaceholder() {
		// Nothing to wait for, and asking would be offering a choice with one
		// answer.
		return opts, opts.Validate()
	}

	wait, err := ask.pick("Wait for the agent to connect?",
		"It reports ready once it has registered with the control plane.", "",
		[]option{
			{value: "wait", label: "wait for it to connect",
				note: "a green install means a working agent"},
			{value: "skip", label: "do not wait",
				note: "the control plane is not reachable yet — install and check later"},
		}, "--wait=false")
	if err != nil {
		return opts, err
	}
	opts.NoWait = wait == "skip"

	// Checked here so a wrong answer is corrected while the operator is still
	// standing here, rather than three minutes into a helm install.
	if err := opts.Validate(); err != nil {
		return opts, err
	}
	return opts, nil
}

// describeCredential says which form was given without printing it.
func describeCredential(opts agent.Options) string {
	switch {
	case opts.UsesExistingSecret():
		return "the Secret " + opts.ExistingSecret + ", already in the cluster"
	case strings.HasPrefix(opts.Token, agent.EnrollmentPrefix):
		return "an enrollment token"
	default:
		return "a per-agent token"
	}
}

// describeNamespaces renders the list, or says that there is none and what that
// means.
func describeNamespaces(opts agent.Options) string {
	if len(opts.ManagedNamespaces) == 0 {
		return "none — the agent can install nowhere until one is added"
	}
	return strings.Join(opts.ManagedNamespaces, ", ")
}

// agentOffer reports whether managing the agent is worth offering.
//
// Whenever there is a cluster, whatever the action was — the same rule the
// kubectl row above follows, and for the same reason it was changed to follow
// it. Gating this on apply read as "the agent belongs to a deploy", when what
// it belongs to is the cluster: somebody who has just planned against a running
// cluster is exactly somebody who may want to look at the agent in it, and the
// cluster is there either way.
//
// Still never a default. It reaches a real cluster, and installing needs a
// credential nobody has lying around.
func agentOffer(cluster bool) bool {
	return cluster
}

// agentInstallFailed reports an install that did not happen, without ending the
// run: the deploy itself succeeded, and the menu is still the right place.
func agentInstallFailed(out io.Writer, err error) {
	if errors.Is(err, errBack) || errors.Is(err, infra.ErrAborted) {
		fmt.Fprintf(out, "\n  Left the agent out. `lerian agent install` puts one in later.\n\n")
		return
	}
	fmt.Fprintf(out, "\n  The agent was not installed: %v\n\n", err)
	fmt.Fprintf(out, "  The cluster is up either way. `lerian agent install` tries again.\n\n")
}
