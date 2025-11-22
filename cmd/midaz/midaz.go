package midaz

import (
	"github.com/lerian-studio/lerian-cli/cmd/midaz/ledger"
	"github.com/spf13/cobra"
)

// MidazCmd represents the midaz command
var MidazCmd = &cobra.Command{
	Use:   "midaz",
	Short: "Midaz ledger management commands",
	Long:  `Manage your Midaz ledger deployments and resources.`,
}

func init() {
	MidazCmd.AddCommand(ledger.LedgerCmd)
}
