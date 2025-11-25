package ledger

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/client"
	"github.com/lerian-studio/lerian-cli/internal/config"
)

var (
	deleteForce bool
)

var deleteCmd = &cobra.Command{
	Use:   "delete <deployment-id>",
	Short: "Delete a ledger deployment",
	Long: `Delete a Midaz ledger deployment.

This will permanently delete the ledger and all associated resources.
Use with caution as this action cannot be undone.

Examples:
  # Delete a ledger (with confirmation prompt)
  lerian ledger delete 7b52ec6d-9388-4975-bf48-58879b576a35

  # Delete without confirmation prompt
  lerian ledger delete 7b52ec6d-9388-4975-bf48-58879b576a35 --force`,
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runDelete,
}

func init() {
	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "Skip confirmation prompt")
}

func runDelete(_ *cobra.Command, args []string) error {
	deploymentID := args[0]

	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get current profile
	profile, err := cfg.GetProfile("")
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}

	// Create API client
	apiClient := client.NewClient(profile.APIURL, profile.APIKey, profile.TenantID)

	// Get deployment info first
	deployment, err := apiClient.GetDeploymentByID(deploymentID)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Confirm deletion unless --force is used
	if !deleteForce {
		color.Yellow("\nWARNING: You are about to delete the following ledger:\n")
		fmt.Printf("  Name:        %s\n", deployment.Name)
		fmt.Printf("  ID:          %s\n", deployment.ID)
		fmt.Printf("  Type:        %s\n", deployment.Type)
		fmt.Printf("  Region:      %s\n", deployment.Region)
		fmt.Printf("  Environment: %s\n", deployment.Environment)
		fmt.Printf("\n")
		color.Red("This action is IRREVERSIBLE. All data will be permanently deleted.\n\n")
		fmt.Printf("Type the ledger name '%s' to confirm deletion: ", deployment.Name)

		var confirmation string
		_, _ = fmt.Scanln(&confirmation)

		if strings.TrimSpace(confirmation) != deployment.Name {
			color.Yellow("\nDeletion canceled. Name did not match.")
			return nil
		}
	}

	// Delete deployment
	fmt.Printf("\nDeleting ledger '%s'...\n", deployment.Name)
	if err := apiClient.DeleteDeployment(deploymentID); err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	color.Green("Ledger '%s' has been deleted successfully", deployment.Name)
	return nil
}
