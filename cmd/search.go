package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/spf13/cobra"
)


var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search documents by name",
	Long: `Search documents by name (case-insensitive substring) across the library.

Combine with --tag to scope results to a tag (requires SSH).`,
	Example: `  remarkable search "meeting"
  remarkable search "PMF" --tag work`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]
		searchAll, _ := cmd.Flags().GetBool("all")
		searchTag, _ := cmd.Flags().GetString("tag")

		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		// list all docs
		docs, err := t.ListDocuments()
		if err != nil {
			outputError(err)
			return err
		}

		// filter trashed unless --all
		if !searchAll {
			filtered := make([]model.Document, 0, len(docs))
			for _, d := range docs {
				if d.Parent != "trash" {
					filtered = append(filtered, d)
				}
			}
			docs = filtered
		}

		// filter by tag if set (requires SSH)
		if searchTag != "" {
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
						if tag == searchTag {
							tagged = append(tagged, d)
							break
						}
					}
				}
				docs = tagged
			}
		}

		// case-insensitive substring match on name
		lowerQuery := strings.ToLower(query)
		tree := model.NewTree(docs)
		results := make([]model.Document, 0)

		for _, d := range docs {
			if !strings.Contains(strings.ToLower(d.Name), lowerQuery) {
				continue
			}
			doc := d
			doc.Path = tree.Path(d.ID)
			results = append(results, doc)
		}

		output(results)
		return nil
	},
}

func init() {
	searchCmd.Flags().Bool("all", false, "include trashed documents")
	searchCmd.Flags().String("tag", "", "filter by document tag (requires SSH)")
	rootCmd.AddCommand(searchCmd)
}
