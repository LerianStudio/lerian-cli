package ledger

import (
	"fmt"

	"github.com/lerian-studio/lerian-cli/internal/client"
	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/kubectl"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec LEDGER_ID [COMMAND...]",
	Short: "Execute SQL commands in the ledger database",
	Long:  `Execute SQL commands directly in the ledger's PostgreSQL database using psql.`,
	Args:  cobra.MinimumNArgs(1),
	RunE:  runExec,
}

func runExec(cmd *cobra.Command, args []string) error {
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

	// Get PostgreSQL pod
	labelSelector := "app.kubernetes.io/name=postgresql"
	pods, err := kubectlClient.GetPods(labelSelector)
	if err != nil {
		return fmt.Errorf("failed to get PostgreSQL pods: %w", err)
	}

	// Build psql command
	psqlCmd := []string{"psql", "-U", "postgres", "-d", "midaz"}

	// If additional arguments provided, execute them as SQL
	if len(args) > 1 {
		psqlCmd = append(psqlCmd, "-c", args[1])
	}

	fmt.Printf("Connecting to PostgreSQL pod: %s\n", pods[0])
	fmt.Printf("Database: midaz\n\n")

	// Execute command
	if err := kubectlClient.ExecCommand(pods[0], psqlCmd); err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}

	return nil
}
