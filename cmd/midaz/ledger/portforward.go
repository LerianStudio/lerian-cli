package ledger

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/client"
	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/kubectl"
)

var (
	localPort  int
	remotePort int
)

var portForwardCmd = &cobra.Command{
	Use:   "port-forward LEDGER_ID",
	Short: "Forward a local port to the ledger service",
	Long:  `Forward a local port to the ledger CRUD API service for direct access to the ledger's REST API.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runPortForward,
}

func init() {
	portForwardCmd.Flags().IntVarP(&localPort, "local-port", "l", 5000, "Local port to forward from")
	portForwardCmd.Flags().IntVarP(&remotePort, "remote-port", "r", 5000, "Remote port to forward to")
}

func runPortForward(cmd *cobra.Command, args []string) error {
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

	// Construct service name from helm release name
	serviceName := fmt.Sprintf("%s-ledger-crud", ledger.HelmReleaseName)

	// Verify service exists
	_, err = kubectlClient.GetService(serviceName)
	if err != nil {
		return fmt.Errorf("failed to find service %s: %w", serviceName, err)
	}

	fmt.Printf("Forwarding localhost:%d -> service/%s:%d\n", localPort, serviceName, remotePort)
	fmt.Printf("Press Ctrl+C to stop port forwarding\n\n")

	// Port forward to the service
	if err := kubectlClient.PortForwardService(serviceName, localPort, remotePort); err != nil {
		return fmt.Errorf("failed to port-forward: %w", err)
	}

	return nil
}
