package agent

import (
	"fmt"
	"sync"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/agent"
	"github.com/lerian-studio/lerian-cli/internal/infracli"
)

var status struct {
	namespace   string
	kubeContext string
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether the agent is installed, and what it is running",
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().StringVar(&status.namespace, "namespace", agent.DefaultNamespace,
		"namespace the agent runs in")
	statusCmd.Flags().StringVar(&status.kubeContext, "kube-context", "",
		"kubeconfig context to read; empty uses the current one")
}

func runStatus(cmd *cobra.Command, _ []string) error {
	helm, err := agent.NewHelm()
	if err != nil {
		return err
	}
	helm.Context = target.Context
	helm.Out = cmd.OutOrStdout()

	var mu sync.Mutex
	spin := infracli.Spin(&mu, cmd.OutOrStdout(), "reading the release")
	release, installed, err := helm.Installed(cmd.Context(), status.namespace)
	spin.Stop()
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	// Where it looked, before what it found: "no agent" means nothing without
	// the cluster it is about.
	fmt.Fprintf(out, "\n%s\n", infracli.Heading(out, "==> "+target.Described()))
	if !installed {
		// Not an error. "Is it there?" is a question whose answer can be no.
		fmt.Fprintf(out, "No agent in %s.\n\n  lerian agent install --control-plane <url> --token <token>\n",
			status.namespace)
		return nil
	}

	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(writer, "release\t%s\n", release.Name)
	fmt.Fprintf(writer, "namespace\t%s\n", release.Namespace)
	fmt.Fprintf(writer, "chart\t%s\n", release.Chart)
	fmt.Fprintf(writer, "app version\t%s\n", release.AppVersion)
	fmt.Fprintf(writer, "revision\t%s\n", release.Revision)
	fmt.Fprintf(writer, "status\t%s\n", release.Status)
	fmt.Fprintf(writer, "updated\t%s\n", release.Updated)
	_ = writer.Flush()

	// A failed release leaves the pod in place on purpose, so there is something
	// to read. Say where to read it rather than making somebody guess.
	if release.Status != "deployed" {
		fmt.Fprintf(out, "\nThe release is %s. What the agent itself says:\n", release.Status)
		fmt.Fprintf(out, "  kubectl -n %s logs deploy/%s\n", release.Namespace, agent.ReleaseName)
	}
	return nil
}
