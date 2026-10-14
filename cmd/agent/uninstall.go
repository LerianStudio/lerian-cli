package agent

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/agent"
	"github.com/lerian-studio/lerian-cli/internal/infracli"
)

var uninstall struct {
	namespace   string
	kubeContext string
	yes         bool
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the agent, keeping its identity",
	Long: `Remove the agent from the cluster.

The Secret ` + agent.IdentitySecretName + ` is kept, which is the chart's own
behavior and the right one: the control plane knows this agent by an identity
it was given once, and a reinstall that threw it away would arrive as a
stranger.`,
	RunE: runUninstall,
}

func init() {
	uninstallCmd.Flags().StringVar(&uninstall.namespace, "namespace", agent.DefaultNamespace,
		"namespace the agent runs in")
	uninstallCmd.Flags().StringVar(&uninstall.kubeContext, "kube-context", "",
		"kubeconfig context to act in; empty uses the current one")
	uninstallCmd.Flags().BoolVar(&uninstall.yes, "yes", false,
		"skip the confirmation; required outside a terminal")
}

func runUninstall(cmd *cobra.Command, _ []string) error {
	helm, err := agent.NewHelm()
	if err != nil {
		return err
	}
	helm.Context = target.Context

	release, installed, err := helm.Installed(cmd.Context(), uninstall.namespace)
	if err != nil {
		return err
	}
	if !installed {
		fmt.Fprintf(cmd.OutOrStdout(), "No agent in %s; nothing to remove.\n", uninstall.namespace)
		return nil
	}

	if !uninstall.yes {
		// Named, because "the agent" is not specific enough when somebody has
		// more than one cluster in their kubeconfig.
		//
		// Asked rather than answered with "re-run with --yes": this command is
		// reached from a menu, where there is no command line to add a flag to,
		// and telling somebody to re-run with a flag they never typed leaves them
		// where they started.
		what := fmt.Sprintf("remove %s (%s) from %s in %s",
			release.Name, release.Chart, release.Namespace, target.Described())
		fmt.Fprintf(cmd.OutOrStdout(), "\n  %s\n  The control plane will stop hearing from it.\n",
			what)
		if err := infracli.ConfirmAgentAction(cmd.Context(), cmd.OutOrStdout(), what); err != nil {
			return err
		}
	}

	if err := helm.Uninstall(cmd.Context(), uninstall.namespace); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nRemoved. %s was kept, so a reinstall is the same agent.\n",
		agent.IdentitySecretName)
	return nil
}
