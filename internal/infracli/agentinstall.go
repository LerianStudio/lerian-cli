package infracli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lerian-studio/lerian-cli/internal/agent"
	"github.com/lerian-studio/lerian-cli/internal/infra"
)

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
	helm.Out = out

	opts, err := askAgentOptions(ask)
	if err != nil {
		return err
	}

	// Said before it is done, because an install reaches a real cluster and the
	// values came from four separate answers.
	fmt.Fprintf(out, "\n%s\n", newStyle(out).bold("==> Installing the Lerian agent"))
	fmt.Fprintf(out, "  namespace          %s\n", opts.Namespace)
	fmt.Fprintf(out, "  control plane      %s\n", opts.ControlPlaneURL)
	fmt.Fprintf(out, "  credential         %s\n", describeCredential(opts))
	fmt.Fprintf(out, "  managed namespaces %s\n\n", describeNamespaces(opts))

	if err := agent.Install(ctx, helm, opts, agent.DefaultTimeout); err != nil {
		return err
	}

	fmt.Fprintf(out, "\n  The agent is installed. It appears in the control plane once it has sent\n")
	fmt.Fprintf(out, "  its first heartbeat:\n")
	fmt.Fprintf(out, "    lerian agent status\n\n")
	return nil
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
	opts := agent.Options{Namespace: agent.DefaultNamespace}

	// Offered rather than asked blind: `lerian auth login` already wrote where
	// the platform is, and the agent connects to that same platform. Enter takes
	// it; a different cluster talking to a different control plane is the case
	// that types one.
	known := agent.ControlPlaneFromProfile("")
	purpose := "The agent connects outward to it. https:// — it sends a bearer token on every request."
	if known != "" {
		purpose += " Enter takes the one you signed in to."
	}
	url, err := ask.ask("Where is the Lerian control plane?", purpose, known, "--control-plane")
	if err != nil {
		return opts, err
	}
	opts.ControlPlaneURL = strings.TrimSpace(url)

	token, err := ask.ask("Enrollment token?",
		"Single-use, from POST /api/tenants/:tenantId/agents/enrollments. "+
			"The agent is told its identity when it redeems one.",
		"", "--token")
	if err != nil {
		return opts, err
	}
	opts.Token = strings.TrimSpace(token)

	namespaces, err := ask.ask("Namespaces the agent may install into?",
		"Comma-separated, and each must already exist. Leave empty to decide later — "+
			"adding one is another install with the same values.",
		"", "--managed-namespaces")
	if err != nil {
		return opts, err
	}
	for _, name := range strings.Split(namespaces, ",") {
		if name = strings.TrimSpace(name); name != "" {
			opts.ManagedNamespaces = append(opts.ManagedNamespaces, name)
		}
	}

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

// agentOffer reports whether installing the agent is worth offering.
//
// Only after an apply, and only with a cluster: an offer to install into
// something that was not built, or was only planned, is a question with no
// answer. Never as a default — it reaches a real cluster and needs a credential
// nobody has lying around.
func agentOffer(done infra.Action, cluster bool) bool {
	return done == infra.ActionApply && cluster
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
