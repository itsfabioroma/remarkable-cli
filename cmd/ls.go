package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/spf13/cobra"
)

// flags
var flagAll bool
var flagTag string

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

		// filter by tag (requires SSH to read .content files)
		if flagTag != "" {
			sshT, sshErr := getSSH()
			if sshErr != nil {
				fmt.Fprintf(os.Stderr, "warning: --tag requires SSH, skipping tag filter\n")
			} else {
				defer sshT.Close()
				tagged := make([]model.Document, 0)
				for _, d := range docs {
					rc, err := sshT.ReadFile(d.ID, "content")
					if err != nil {
						continue
					}
					var raw map[string]any
					json.NewDecoder(rc).Decode(&raw)
					rc.Close()

					for _, tag := range getStringSlice(raw, "tags") {
						if tag == flagTag {
							tagged = append(tagged, d)
							break
						}
					}
				}
				docs = tagged
			}
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
	lsCmd.Flags().StringVar(&flagTag, "tag", "", "filter by document tag")
	rootCmd.AddCommand(lsCmd)
}
