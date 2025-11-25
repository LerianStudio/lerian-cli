package ledger

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/client"
	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/kubectl"
)

var (
	follow bool
	tail   int
)

var logsCmd = &cobra.Command{
	Use:   "logs LEDGER_ID",
	Short: "View ledger service logs",
	Long:  `View logs from the ledger service pods in Kubernetes.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().IntVarP(&tail, "tail", "t", 50, "Number of lines to show from the end of the logs")
}

func runLogs(cmd *cobra.Command, args []string) error {
	ledgerID := args[0]

	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get profile from flags or use current
	profileName, _ := cmd.Flags().GetString("profile")
	if profileName == "" {
		profileName = cfg.CurrentProfile
	}

	profile, err := cfg.GetProfile(profileName)
	if err != nil {
		return fmt.Errorf("profile error: %w. Please run 'lerian auth login' first", err)
	}

	// Create client
	apiClient := client.NewClient(profile.APIURL, profile.APIKey, profile.TenantID)

	// Get ledger to get namespace
	ledger, err := apiClient.GetLedger(ledgerID)
	if err != nil {
		return fmt.Errorf("failed to get ledger: %w", err)
	}

	if ledger.HelmNamespace == "" {
		return fmt.Errorf("ledger does not have a Kubernetes namespace configured")
	}

	// Create kubectl client
	kubectlClient := kubectl.NewClient(ledger.HelmNamespace)

	// Get pod names for the ledger
	labelSelector := fmt.Sprintf("app.kubernetes.io/instance=%s", ledger.HelmReleaseName)
	pods, err := kubectlClient.GetPods(labelSelector)
	if err != nil {
		return fmt.Errorf("failed to get pods: %w", err)
	}

	fmt.Printf("Found %d pod(s) for ledger %s\n", len(pods), ledgerID)
	fmt.Printf("Showing logs from pod: %s\n\n", pods[0])

	// Get logs from the first pod
	if err := kubectlClient.GetLogs(pods[0], follow, tail); err != nil {
		return fmt.Errorf("failed to get logs: %w", err)
	}

	return nil
}
