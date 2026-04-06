package cmd

import (
	"github.com/spf13/cobra"
)

var lsCmd = &cobra.Command{
	Use:   "ls [path]",
	Short: "List documents and folders",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		docs, err := t.ListDocuments()
		if err != nil {
			outputError(err)
			return err
		}

		// if a path is given, filter to that folder
		if len(args) > 0 {
			// TODO: resolve path and filter children
			_ = args[0]
		}

		output(docs)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(lsCmd)
}
