package cmd

import (
	"github.com/spf13/cobra"
)

// auth is an alias for connect --cloud-only
var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with reMarkable Cloud (alias for connect --cloud-only)",
	RunE: func(cmd *cobra.Command, args []string) error {
		connectCmd.Flags().Set("cloud-only", "true")
		return connectCmd.RunE(connectCmd, args)
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
}
