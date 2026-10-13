package agent

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/agent"
)

var install struct {
	opts        agent.Options
	kubeContext string
	timeout     time.Duration
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the agent into the cluster kubectl points at",
	Long: `Install the Lerian agent.

The credential is either a single-use enrollment token, which carries no
identity and is safe to hand to a cluster that does not exist yet, or a
per-agent token, which needs --agent-id with it. Both reach the cluster as a
Secret; --existing-secret is the way the credential never passes through this
process at all.

Every namespace in --managed-namespaces must already exist: the chart creates a
Role and a RoleBinding in each, and Kubernetes has nowhere to put them
otherwise.`,
	Example: `  lerian agent install --control-plane https://cp.example.com --token lerian_enroll_...
  lerian agent install --control-plane https://cp.example.com --existing-secret agent-creds
  lerian agent install --control-plane https://cp.example.com --token ... --managed-namespaces apps,data`,
	RunE: runInstall,
}

func init() {
	flags := installCmd.Flags()
	flags.StringVar(&install.opts.ControlPlaneURL, "control-plane", "",
		"base URL of the Lerian control plane")
	flags.StringVar(&install.opts.Token, "token", "",
		"enrollment or per-agent token from the control plane")
	flags.StringVar(&install.opts.AgentID, "agent-id", "",
		"agent id, required with a per-agent token and refused with an enrollment one")
	flags.StringVar(&install.opts.ExistingSecret, "existing-secret", "",
		"name a Secret already in the cluster instead of passing a token")
	flags.StringSliceVar(&install.opts.ManagedNamespaces, "managed-namespaces", nil,
		"namespaces the agent may install into; each must already exist")
	flags.StringVar(&install.opts.Namespace, "namespace", agent.DefaultNamespace,
		"namespace the agent itself runs in")
	flags.StringVar(&install.opts.ChartVersion, "chart-version", "",
		"pin the chart; empty installs the newest published version")
	flags.BoolVar(&install.opts.AllowInsecureHTTP, "allow-insecure-http", false,
		"permit a cleartext control plane URL, for an isolated dev cluster only")
	flags.StringVar(&install.opts.ImageRepository, "image-repository", "",
		"pull the agent image from somewhere other than the chart's default")
	flags.StringVar(&install.opts.ImageDigest, "image-digest", "",
		"pin the image by digest")
	flags.StringVar(&install.opts.AllowedRegistries, "allowed-registries", "",
		"registries the agent accepts for its own self-update; set it alongside --image-repository")
	flags.StringSliceVar(&install.opts.ImagePullSecrets, "image-pull-secrets", nil,
		"Secrets for a registry that refuses anonymous reads")
	flags.StringVar(&install.kubeContext, "kube-context", "",
		"kubeconfig context to install into; empty uses the current one")
	flags.DurationVar(&install.timeout, "timeout", agent.DefaultTimeout,
		"how long to wait for the agent to become ready")

	_ = installCmd.MarkFlagRequired("control-plane")
}

func runInstall(cmd *cobra.Command, _ []string) error {
	helm, err := agent.NewHelm()
	if err != nil {
		return err
	}
	helm.Context = install.kubeContext
	helm.Out = cmd.OutOrStdout()

	if err := agent.Install(cmd.Context(), helm, install.opts, install.timeout); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nThe agent is installed in %s.\n", install.opts.Namespace)
	fmt.Fprintf(cmd.OutOrStdout(), "It appears in the control plane once it sends its first heartbeat:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  lerian agent status\n")
	return nil
}
