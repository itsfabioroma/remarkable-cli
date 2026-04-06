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

		doc := matches[0]

		// build result
		result := map[string]any{
			"id":           doc.ID,
			"name":         doc.Name,
			"type":         doc.Type,
			"path":         tree.Path(doc.ID),
			"fileType":     doc.FileType,
			"pageCount":    doc.PageCount,
			"lastModified": doc.LastModified,
			"pinned":       doc.Pinned,
			"parent":       doc.Parent,
		}

		// try reading .content for tags (SSH only)
		if sshT, ok := t.(*transport.SSHTransport); ok {
			_, rawContent, err := readContent(sshT, doc.ID)
			if err == nil {
				var raw map[string]any
				json.Unmarshal(rawContent, &raw)
				tags := getStringSlice(raw, "tags")
				if tags != nil {
					result["tags"] = tags
				}
			}
		}

		// human-readable output for terminals
		if !flagJSON && isTerminal() {
			printDocInfo(result)
			return nil
		}

		output(result)
		return nil
	},
}

// printDocInfo renders human-readable document info
func printDocInfo(result map[string]any) {
	fmt.Printf("Name:          %v\n", result["name"])
	fmt.Printf("Path:          %v\n", result["path"])
	fmt.Printf("Type:          %v\n", result["type"])

	if pc, ok := result["pageCount"].(int); ok && pc > 0 {
		fmt.Printf("Pages:         %d\n", pc)
	}
	if ft, ok := result["fileType"].(string); ok && ft != "" {
		fmt.Printf("File type:     %s\n", ft)
	}

	fmt.Printf("Last modified: %v\n", result["lastModified"])

	if tags, ok := result["tags"].([]string); ok && len(tags) > 0 {
		fmt.Printf("Tags:          %s\n", strings.Join(tags, ", "))
	}
	if pinned, ok := result["pinned"].(bool); ok && pinned {
		fmt.Printf("Pinned:        yes\n")
	}

	fmt.Printf("ID:            %v\n", result["id"])
}

func init() { rootCmd.AddCommand(infoCmd) }
