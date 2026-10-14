package agent

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/agent"
)

var upgrade struct {
	version     string
	namespace   string
	kubeContext string
	timeout     time.Duration
	wait        bool
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Move the installed agent to another chart version",
	Long: `Upgrade the agent in place.

Every other value is kept: helm reuses the release's own, so the credential and
the managed namespaces do not have to be given again. To change those, run
install, which is the same operation with new values.`,
	Example: `  lerian agent upgrade                      # to the newest published chart
  lerian agent upgrade --chart-version 1.0.2`,
	RunE: runUpgrade,
}

func init() {
	upgradeCmd.Flags().StringVar(&upgrade.version, "chart-version", "",
		"version to move to; empty takes the newest published")
	upgradeCmd.Flags().StringVar(&upgrade.namespace, "namespace", agent.DefaultNamespace,
		"namespace the agent runs in")
	upgradeCmd.Flags().StringVar(&upgrade.kubeContext, "kube-context", "",
		"kubeconfig context to act in; empty uses the current one")
	upgradeCmd.Flags().DurationVar(&upgrade.timeout, "timeout", agent.DefaultTimeout,
		"how long to wait for the agent to become ready")
	upgradeCmd.Flags().BoolVar(&upgrade.wait, "wait", true,
		"wait for the agent to become ready; skipped automatically when it was not ready to begin with")
}

func runUpgrade(cmd *cobra.Command, _ []string) error {
	helm, err := agent.NewHelm()
	if err != nil {
		return err
	}
	helm.Context = target.Context

	// Refused rather than turned into an install. An upgrade of something that
	// is not there would have to invent a credential, and inventing one is how
	// an agent arrives unable to authenticate.
	release, installed, err := helm.Installed(cmd.Context(), upgrade.namespace)
	if err != nil {
		return err
	}
	if !installed {
		return fmt.Errorf("no agent in %s to upgrade. `lerian agent install` puts one there",
			upgrade.namespace)
	}

	// An agent that is not ready now will not be made ready by a new chart, so
	// waiting for it would spend the timeout to report a failure that is really
	// the state it was already in. status says which case this is.
	noWait := !upgrade.wait || release.Status != "deployed"

	if err := helm.Reuse(cmd.Context(), upgrade.version, upgrade.namespace, upgrade.timeout, noWait); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nUpgraded from %s.\n", release.Chart)
	return nil
}
