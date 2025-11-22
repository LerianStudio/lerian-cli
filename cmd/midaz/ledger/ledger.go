package ledger

import (
	"github.com/spf13/cobra"
)

// LedgerCmd represents the ledger command
var LedgerCmd = &cobra.Command{
	Use:   "ledger",
	Short: "Ledger management commands",
	Long:  `Manage your Midaz ledger deployments.`,
}

func init() {
	LedgerCmd.AddCommand(createCmd)
	LedgerCmd.AddCommand(listCmd)
	LedgerCmd.AddCommand(describeCmd)
	LedgerCmd.AddCommand(deleteCmd)
	LedgerCmd.AddCommand(versionsCmd)
	LedgerCmd.AddCommand(logsCmd)
	LedgerCmd.AddCommand(portForwardCmd)
	LedgerCmd.AddCommand(backupCmd)
	LedgerCmd.AddCommand(execCmd)
	LedgerCmd.AddCommand(eventsCmd)
}
