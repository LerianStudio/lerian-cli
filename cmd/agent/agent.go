// Package agent is the terminal face of installing the Lerian agent.
//
// Two ways in, one installation. These commands are the explicit one, for a
// cluster that already exists — frequently one this CLI never created. The
// other is the offer at the end of a cluster deploy, which calls the same
// internal/agent code with the values that run already knows.
package agent

import (
	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/infra"
	"github.com/lerian-studio/lerian-cli/internal/infracli"
)

// target is the cluster chosen for this session, once, by PersistentPreRunE.
// Subcommands read it rather than each deciding for itself.
var target infracli.AgentTarget

// AgentCmd groups the agent's lifecycle.
var AgentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Install and manage the Lerian agent in a Kubernetes cluster",
	Long: `Manage the Lerian agent.

The agent runs inside your cluster, connects outward to the Lerian control
plane, and runs the Helm operations it is given. Nothing reaches into the
cluster from outside.

It installs into whatever cluster your kubeconfig points at, so it does not
have to be a cluster this CLI created.`,
}

// PersistentPreRunE settles which cluster before any subcommand runs.
//
// Every one of these reaches a real cluster, and the old answer to "which one?"
// was whatever the kubeconfig happened to point at — an answer nobody chose and
// nothing on screen stated. The checks, the account and the cluster are the
// same three steps a run of `infra` walks, for the same reason.
//
// Skipped when --kube-context names one outright, which is the scripted case:
// somebody who passes a context has already decided, and there is nothing to
// ask. Skipped outside a terminal too, where asking is impossible and the
// current context is the documented behavior.
func preflight(cmd *cobra.Command, _ []string) error {
	if chosen, _ := cmd.Flags().GetString("kube-context"); chosen != "" {
		target = infracli.AgentTarget{Context: chosen}
		return nil
	}
	if carried, ok := infracli.CarriedAgentTarget(); ok {
		// The deploy that just happened answered all of this. Asking again, in the
		// same session, about the cluster whose creation is still on screen, is a
		// question with an answer nobody needs to give twice.
		target = carried
		return nil
	}
	if !infracli.Interactive() {
		return nil
	}

	chosen, err := infracli.PrepareAgent(cmd.Context(), cmd.OutOrStdout(), infra.CLIClusters{})
	if err != nil {
		return err
	}
	target = chosen
	return nil
}

func init() {
	AgentCmd.PersistentPreRunE = preflight
	AgentCmd.AddCommand(installCmd)
	AgentCmd.AddCommand(upgradeCmd)
	AgentCmd.AddCommand(statusCmd)
	AgentCmd.AddCommand(uninstallCmd)
}
