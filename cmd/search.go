package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/spf13/cobra"
)

// searchResult is the JSON-friendly output for each match
type searchResult struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	Type         string `json:"type"`
	FileType     string `json:"fileType,omitempty"`
	PageCount    int    `json:"pageCount,omitempty"`
	LastModified string `json:"lastModified"`
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search documents by name",
	Long: `Search documents by name across the library.

  remarkable search "meeting"         # fuzzy name search
  remarkable search "PMF" --tag work  # search within tagged docs`,
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
		var results []searchResult

		for _, d := range docs {
			if !strings.Contains(strings.ToLower(d.Name), lowerQuery) {
				continue
			}

			results = append(results, searchResult{
				ID:           d.ID,
				Name:         d.Name,
				Path:         tree.Path(d.ID),
				Type:         string(d.Type),
				FileType:     d.FileType,
				PageCount:    d.PageCount,
				LastModified: d.LastModified.Format("2006-01-02T15:04:05Z"),
			})
		}

		// output results
		if len(results) == 0 {
			output([]searchResult{})
			return nil
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
