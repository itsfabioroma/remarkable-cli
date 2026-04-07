package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/itsfabioroma/remarkable-cli/pkg/transport"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show detailed info about a document",
	Long: `Show detailed info about a single document or folder.

Returns the same JSON shape as ls for one document, plus a tree-resolved path and tags.`,
	Example: `  remarkable info "My Notes"
  remarkable --json info "Meeting"`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		// find the document
		docs, err := t.ListDocuments()
		if err != nil {
			outputError(err)
			return err
		}

		tree := model.NewTree(docs)
		matches := tree.Find(args[0])
		if len(matches) == 0 {
			err := model.NewCLIError(model.ErrNotFound, t.Name(), fmt.Sprintf("%q not found", args[0]))
			outputError(err)
			return err
		}
		if len(matches) > 1 {
			err := model.NewCLIError(model.ErrConflict, t.Name(), fmt.Sprintf("ambiguous: %d docs named %q", len(matches), args[0]))
			outputError(err)
			return err
		}

		// canonical doc shape — same as ls, plus path + tags
		doc := *matches[0]
		doc.Path = tree.Path(doc.ID)

		// try reading .content for tags (SSH only)
		if sshT, ok := t.(*transport.SSHTransport); ok {
			_, rawContent, err := readContent(sshT, doc.ID)
			if err == nil {
				var raw map[string]any
				json.Unmarshal(rawContent, &raw)
				if tags := getStringSlice(raw, "tags"); tags != nil {
					doc.Tags = tags
				}
			}
		}

		// human-readable output for terminals
		if !flagJSON && isTerminal() {
			printDocInfo(&doc)
			return nil
		}

		output(doc)
		return nil
	},
}

// printDocInfo renders human-readable document info
func printDocInfo(d *model.Document) {
	fmt.Printf("Name:          %s\n", d.Name)
	fmt.Printf("Path:          %s\n", d.Path)
	fmt.Printf("Type:          %s\n", d.Type)

	if d.PageCount > 0 {
		fmt.Printf("Pages:         %d\n", d.PageCount)
	}
	if d.FileType != "" {
		fmt.Printf("File type:     %s\n", d.FileType)
	}

	fmt.Printf("Last modified: %s\n", d.LastModified)

	if len(d.Tags) > 0 {
		fmt.Printf("Tags:          %s\n", strings.Join(d.Tags, ", "))
	}
	if d.Pinned {
		fmt.Printf("Pinned:        yes\n")
	}

	fmt.Printf("ID:            %s\n", d.ID)
}

func init() { rootCmd.AddCommand(infoCmd) }
