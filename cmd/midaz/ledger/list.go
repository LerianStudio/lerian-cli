package ledger

import (
	"encoding/json"
	"fmt"

	"github.com/lerian-studio/lerian-cli/internal/client"
	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/output"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all ledgers",
	Long:  `List all ledger deployments in your account.`,
	RunE:  runList,
}

func runList(cmd *cobra.Command, _ []string) error {
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

	// List ledgers
	ledgers, err := apiClient.ListLedgers()
	if err != nil {
		return fmt.Errorf("failed to list ledgers: %w", err)
	}

	// Get output format
	outputFormat, _ := cmd.Flags().GetString("output")
	printer := output.NewPrinter(outputFormat)

	// Convert to interface{} for printer
	var ledgersData []interface{}
	jsonData, _ := json.Marshal(ledgers)
	_ = json.Unmarshal(jsonData, &ledgersData)

	// Print results
	if err := printer.PrintLedgerList(ledgersData); err != nil {
		return fmt.Errorf("failed to print ledgers: %w", err)
	}

	return nil
}
