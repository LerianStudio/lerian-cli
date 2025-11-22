package ledger

import (
	"encoding/json"
	"fmt"

	"github.com/lerian-studio/lerian-cli/internal/client"
	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/output"
	"github.com/spf13/cobra"
)

var describeCmd = &cobra.Command{
	Use:   "describe LEDGER_ID",
	Short: "Describe a ledger",
	Long:  `Get detailed information about a specific ledger deployment.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runDescribe,
}

func runDescribe(cmd *cobra.Command, args []string) error {
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

	// Get ledger
	ledger, err := apiClient.GetLedger(ledgerID)
	if err != nil {
		return fmt.Errorf("failed to get ledger: %w", err)
	}

	// Get output format
	outputFormat, _ := cmd.Flags().GetString("output")
	printer := output.NewPrinter(outputFormat)

	// Convert to interface{} for printer
	var ledgerData map[string]interface{}
	jsonData, _ := json.Marshal(ledger)
	json.Unmarshal(jsonData, &ledgerData)

	// Print results
	if err := printer.PrintLedgerDetails(ledgerData); err != nil {
		return fmt.Errorf("failed to print ledger details: %w", err)
	}

	return nil
}
