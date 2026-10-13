// Package agent is the terminal face of installing the Lerian agent.
//
// Two ways in, one installation. These commands are the explicit one, for a
// cluster that already exists — frequently one this CLI never created. The
// other is the offer at the end of a cluster deploy, which calls the same
// internal/agent code with the values that run already knows.
package agent

import (
	"github.com/spf13/cobra"
)

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

func init() {
	AgentCmd.AddCommand(installCmd)
	AgentCmd.AddCommand(upgradeCmd)
	AgentCmd.AddCommand(statusCmd)
	AgentCmd.AddCommand(uninstallCmd)
}
