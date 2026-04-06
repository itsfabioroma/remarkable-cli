package cmd

import (
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/spf13/cobra"
)

// --all flag to include trashed docs
var flagAll bool

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

		// filter out trashed docs unless --all is set
		if !flagAll {
			filtered := make([]model.Document, 0, len(docs))
			for _, d := range docs {
				if d.Parent != "trash" {
					filtered = append(filtered, d)
				}
			}
			docs = filtered
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
	lsCmd.Flags().BoolVarP(&flagAll, "all", "a", false, "include trashed documents")
	rootCmd.AddCommand(lsCmd)
}
