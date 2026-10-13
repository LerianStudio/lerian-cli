package agent

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/agent"
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
		"skip the confirmation")
}

func runUninstall(cmd *cobra.Command, _ []string) error {
	helm, err := agent.NewHelm()
	if err != nil {
		return err
	}
	helm.Context = target.Context
	helm.Out = cmd.OutOrStdout()

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
		fmt.Fprintf(cmd.OutOrStdout(),
			"This removes %s (%s) from namespace %s.\nThe control plane will stop hearing from it.\n"+
				"Re-run with --yes to confirm.\n",
			release.Name, release.Chart, release.Namespace)
		return nil
	}

	if err := helm.Uninstall(cmd.Context(), uninstall.namespace); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nRemoved. %s was kept, so a reinstall is the same agent.\n",
		agent.IdentitySecretName)
	return nil
}
