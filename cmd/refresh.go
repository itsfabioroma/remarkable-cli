package cmd

import (
	"github.com/spf13/cobra"
)

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Restart the device UI so recent changes become visible",
	Long: `Restart xochitl (the reMarkable UI) over SSH so any filesystem changes — uploads, deletes, page edits — become visible immediately.`,
	Example: `  remarkable refresh`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sshT, err := getSSH()
		if err != nil {
			outputError(err)
			return err
		}
		defer sshT.Close()

		// restart xochitl to pick up filesystem changes
		if err := sshT.RestartUI(); err != nil {
			outputError(err)
			return err
		}

		output(map[string]any{"status": "refreshed"})
		return nil
	},
}

func init() { rootCmd.AddCommand(refreshCmd) }
