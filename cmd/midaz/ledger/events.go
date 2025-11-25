package ledger

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/client"
	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/kubectl"
)

var eventsCmd = &cobra.Command{
	Use:   "events LEDGER_ID",
	Short: "View Kubernetes events for the ledger",
	Long:  `View Kubernetes events for the ledger namespace to troubleshoot deployment issues.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runEvents,
}

func runEvents(cmd *cobra.Command, args []string) error {
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

	fmt.Printf("Kubernetes events for ledger %s (namespace: %s):\n\n", ledgerID, ledger.HelmNamespace)

	// Create kubectl client
	kubectlClient := kubectl.NewClient(ledger.HelmNamespace)

	// Get events
	if err := kubectlClient.GetEvents(); err != nil {
		return fmt.Errorf("failed to get events: %w", err)
	}

	return nil
}
