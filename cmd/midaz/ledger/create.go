package ledger

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/lerian-studio/lerian-cli/internal/client"
	"github.com/lerian-studio/lerian-cli/internal/config"
)

var (
	// Required flags
	createName    string
	createRegion  string
	createEnv     string
	createAgentID string

	// Optional flags
	createMode         string
	createSize         string
	createTPS          int
	createMultiAZ      bool
	createSandbox      bool
	createAppVersion   string
	createChartVersion string
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new ledger deployment",
	Long: `Create a new Midaz ledger deployment in SaaS, private, or sandbox mode.

Examples:
  # Create a SaaS ledger
  lerian ledger create --name my-ledger --region us-east-1 --env dev

  # Create a private ledger
  lerian ledger create --name prod-ledger --mode private --region us-west-2 --env prod --size production

  # Create a sandbox ledger (for trials)
  lerian ledger create --name trial-ledger --sandbox --region us-east-1`,
	SilenceUsage: true,
	PreRunE:      preRunCreate,
	RunE:         runCreate,
}

func init() {
	// Required flags
	createCmd.Flags().StringVar(&createName, "name", "", "Name of the ledger (required, 3-100 chars)")
	createCmd.Flags().StringVar(&createRegion, "region", "", "Region for deployment (required)")
	createCmd.Flags().StringVar(&createEnv, "env", "",
		"Environment type: dev, staging, prod (required unless using --sandbox)")
	createCmd.Flags().StringVar(&createAgentID, "agent-id", "",
		"Agent ID (required for private mode, optional for SaaS)")
	_ = createCmd.MarkFlagRequired("name")
	_ = createCmd.MarkFlagRequired("region")

	// Optional flags
	createCmd.Flags().StringVar(&createMode, "mode", "saas", "Deployment mode: saas, private")
	createCmd.Flags().StringVar(&createSize, "size", "test", "Ledger size: test, staging, production")
	createCmd.Flags().IntVar(&createTPS, "tps", 0, "Transactions per second (10-10000, default based on size)")
	createCmd.Flags().BoolVar(&createMultiAZ, "multi-az", false, "Enable multi-AZ deployment")
	createCmd.Flags().BoolVar(&createSandbox, "sandbox", false, "Create sandbox ledger (auto-expires in 7 days)")
	createCmd.Flags().StringVar(&createAppVersion, "app-version", "", "Specific app version to deploy")
	createCmd.Flags().StringVar(&createChartVersion, "chart-version", "", "Specific Helm chart version")
}

func preRunCreate(cmd *cobra.Command, _ []string) error {
	// Apply sandbox overrides before validation
	if createSandbox {
		// Check for conflicting flags
		if cmd.Flags().Changed("mode") && createMode != "saas" {
			return fmt.Errorf("--sandbox flag conflicts with --mode=%s (sandbox requires saas mode)", createMode)
		}
		if cmd.Flags().Changed("size") && createSize != "test" {
			return fmt.Errorf("--sandbox flag conflicts with --size=%s (sandbox requires test size)", createSize)
		}
		if cmd.Flags().Changed("env") && createEnv != "dev" {
			return fmt.Errorf("--sandbox flag conflicts with --env=%s (sandbox requires dev environment)", createEnv)
		}

		createMode = "saas"
		createSize = "test"
		createTPS = 10
		createEnv = "dev"
		color.Yellow("INFO: Sandbox mode: Using saas/test/dev configuration")
	}
	return nil
}

func runCreate(cmd *cobra.Command, args []string) error {
	// Validate env is provided (unless sandbox mode already set it)
	if createEnv == "" && !createSandbox {
		return fmt.Errorf("--env flag is required (unless using --sandbox)")
	}

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

	// Validate inputs
	if err := validateCreateInputs(); err != nil {
		return err
	}

	// Validate agent-id requirement based on mode
	if createMode == "private" && createAgentID == "" {
		return fmt.Errorf("--agent-id is required for private mode deployments")
	}

	// Set default TPS based on size if not specified
	if createTPS == 0 {
		createTPS = getDefaultTPS(createSize)
	}

	// Parse agent ID
	var agentID *uuid.UUID
	if createAgentID != "" {
		parsed, err := uuid.Parse(createAgentID)
		if err != nil {
			return fmt.Errorf("invalid agent-id format: %w", err)
		}
		agentID = &parsed
	}

	// Create API client
	apiClient := client.NewClient(profile.APIURL, profile.APIKey, profile.TenantID)

	// Build request
	deploymentType := client.DeploymentTypeSaaS
	if createMode == "private" {
		deploymentType = client.DeploymentTypeSingleTenant
	}

	req := &client.CreateDeploymentRequest{
		Name:        createName,
		Type:        deploymentType,
		Region:      createRegion,
		Size:        &createSize,
		TPS:         &createTPS,
		Environment: createEnv,
		MultiAZ:     createMultiAZ,
		Sandbox:     createSandbox,
		AgentID:     agentID,
	}

	// TODO(review): Add semantic version validation for app-version and chart-version
	// Reported by: business-logic-reviewer on 2025-11-17
	// Severity: Medium
	if createAppVersion != "" {
		req.AppVersion = &createAppVersion
	}
	if createChartVersion != "" {
		req.ChartVersion = &createChartVersion
	}

	// Create deployment
	// TODO(review): Add structured logging/audit trail for deployment operations
	// Reported by: code-reviewer on 2025-11-17
	// Severity: Medium
	fmt.Printf("Creating ledger '%s' in region '%s'...\n", createName, createRegion)
	deployment, err := apiClient.CreateDeployment(req)
	if err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	fmt.Printf("Deployment created with ID: %s\n", deployment.ID)
	fmt.Println("Waiting for provisioning to complete...")

	// Poll for status
	finalDeployment, err := pollDeploymentStatus(apiClient, deployment.ID.String())
	if err != nil {
		return err
	}

	// Show results
	displayDeploymentResult(finalDeployment, createSandbox)

	return nil
}

func validateCreateInputs() error {
	// Validate name - trim and check for whitespace-only names
	trimmedName := strings.TrimSpace(createName)
	if trimmedName == "" {
		return fmt.Errorf("name cannot be empty or contain only whitespace")
	}
	if trimmedName != createName {
		return fmt.Errorf("name cannot have leading or trailing whitespace")
	}
	if len(createName) < 3 || len(createName) > 100 {
		return fmt.Errorf("name must be between 3 and 100 characters")
	}

	// Validate region format
	if err := validateRegion(createRegion, createMode); err != nil {
		return err
	}

	// Validate environment
	validEnvs := map[string]bool{"dev": true, "staging": true, "prod": true}
	if !validEnvs[createEnv] {
		return fmt.Errorf("env must be one of: dev, staging, prod")
	}

	// Validate mode
	validModes := map[string]bool{"saas": true, "private": true}
	if !validModes[createMode] {
		return fmt.Errorf("mode must be one of: saas, private")
	}

	// Validate size
	validSizes := map[string]bool{"test": true, "staging": true, "production": true}
	if !validSizes[createSize] {
		return fmt.Errorf("size must be one of: test, staging, production")
	}

	// Validate TPS range
	// FIXME(nitpick): Add warning for high TPS values (>5000) about rate limiting and costs
	// Reported by: business-logic-reviewer on 2025-11-17
	// Severity: Low
	if createTPS != 0 && (createTPS < 10 || createTPS > 10000) {
		return fmt.Errorf("tps must be between 10 and 10000")
	}

	// TODO(review): Add validation to prevent multi-AZ with test size
	// Reported by: business-logic-reviewer on 2025-11-17
	// Severity: Medium
	// if createMultiAZ && createSize == "test" {
	//     return fmt.Errorf("multi-az is not available for test size ledgers")
	// }

	return nil
}

func validateRegion(region, mode string) error {
	// Basic region format validation
	// AWS regions follow pattern: us-east-1, eu-west-2, ap-south-1, etc.
	// Private regions should use "private-" prefix
	awsRegionPattern := regexp.MustCompile(`^[a-z]{2}-[a-z]+-\d+$`)
	privateRegionPattern := regexp.MustCompile(`^private-[a-z0-9-]+$`)

	if mode == "private" {
		// Private mode requires "private-" prefix
		if !privateRegionPattern.MatchString(region) {
			return fmt.Errorf("private mode requires region with 'private-' prefix (e.g., private-us-east-1)")
		}
	} else {
		// SaaS mode should use standard AWS region format or private- prefix
		if !awsRegionPattern.MatchString(region) && !privateRegionPattern.MatchString(region) {
			return fmt.Errorf(
				"region must match AWS format (e.g., us-east-1) or private format (e.g., private-us-east-1)",
			)
		}
	}

	return nil
}

func getDefaultTPS(size string) int {
	switch size {
	case "test":
		return 10
	case "staging":
		return 100
	case "production":
		return 1000
	default:
		return 10
	}
}

func pollDeploymentStatus(apiClient *client.Client, deploymentID string) (*client.Deployment, error) {
	// FIXME(nitpick): Extract polling parameters to package-level constants
	// Reported by: code-reviewer on 2025-11-17
	// Severity: Low
	maxAttempts := 60 // 5 minutes (5 seconds * 60)
	for i := 0; i < maxAttempts; i++ {
		deployment, err := apiClient.GetDeploymentByID(deploymentID)
		if err != nil {
			// TODO(review): Include deployment ID and attempt number in error context
			// Reported by: code-reviewer on 2025-11-17
			// Severity: Medium
			return nil, fmt.Errorf("failed to check deployment status: %w", err)
		}

		switch deployment.Status {
		case client.DeploymentStatusAvailable:
			return deployment, nil
		case client.DeploymentStatusFailed:
			errMsg := "unknown error"
			if deployment.ErrorMessage != nil {
				errMsg = *deployment.ErrorMessage
			}
			return nil, fmt.Errorf("deployment failed: %s", errMsg)
		case client.DeploymentStatusProvisioning:
			// Continue polling
			if i%6 == 0 { // Print every 30 seconds
				fmt.Printf("  Status: %s (elapsed: %ds)\n", deployment.Status, i*5)
			}
			time.Sleep(5 * time.Second)
		default:
			return nil, fmt.Errorf("unexpected deployment status: %s", deployment.Status)
		}
	}

	return nil, fmt.Errorf("deployment timeout after 5 minutes")
}

func displayDeploymentResult(deployment *client.Deployment, isSandbox bool) {
	color.Green("\nLedger deployment complete!")
	fmt.Printf("  Name: %s\n", deployment.Name)
	fmt.Printf("  ID: %s\n", deployment.ID)
	fmt.Printf("  Status: %s\n", deployment.Status)
	fmt.Printf("  Type: %s\n", deployment.Type)
	fmt.Printf("  Region: %s\n", deployment.Region)
	fmt.Printf("  Environment: %s\n", deployment.Environment)

	if deployment.Endpoint != nil {
		fmt.Printf("  Endpoint: %s\n", *deployment.Endpoint)
	}

	if isSandbox {
		expirationDate := deployment.CreatedAt.Add(7 * 24 * time.Hour)
		color.Yellow("\nWARNING: Sandbox ledgers expire after 7 days")
		fmt.Printf("  Expiration date: %s\n", expirationDate.Format("2006-01-02 15:04:05 MST"))
	}

	fmt.Println("\nUse 'lerian ledger describe " + deployment.ID.String() + "' for more details")
}
