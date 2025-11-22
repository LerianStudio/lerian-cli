package ledger

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lerian-studio/lerian-cli/internal/client"
	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/kubectl"
	"github.com/spf13/cobra"
)

var (
	backupDir string
)

var backupCmd = &cobra.Command{
	Use:   "backup LEDGER_ID",
	Short: "Backup ledger PostgreSQL database",
	Long:  `Create a backup of the ledger's PostgreSQL database using pg_dump.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runBackup,
}

func init() {
	backupCmd.Flags().StringVarP(&backupDir, "output-dir", "d", "./backups", "Directory to store backup files")
}

func runBackup(cmd *cobra.Command, args []string) error {
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

	// Create backup directory if it doesn't exist
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Generate backup filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	backupFile := filepath.Join(backupDir, fmt.Sprintf("%s-backup-%s.sql", ledgerID, timestamp))

	fmt.Printf("Creating backup of ledger %s...\n", ledgerID)
	fmt.Printf("PostgreSQL pod: %s\n", pods[0])
	fmt.Printf("Output file: %s\n\n", backupFile)

	// Execute pg_dump
	pgDumpCmd := []string{
		"pg_dump",
		"-U", "postgres",
		"-d", "midaz",
		"-f", "/tmp/backup.sql",
	}

	if err := kubectlClient.ExecCommand(pods[0], pgDumpCmd); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Copy backup file from pod
	copyCmd := []string{
		"kubectl", "cp",
		fmt.Sprintf("%s/%s:/tmp/backup.sql", ledger.HelmNamespace, pods[0]),
		backupFile,
	}

	if err := kubectlClient.ExecCommand("", copyCmd); err != nil {
		return fmt.Errorf("failed to copy backup file: %w", err)
	}

	fmt.Printf("Backup completed successfully: %s\n", backupFile)
	return nil
}
