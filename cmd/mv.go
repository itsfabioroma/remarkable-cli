package cmd

import (
	"fmt"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/spf13/cobra"
)

var mvCmd = &cobra.Command{
	Use:   "mv <source> <destination>",
	Short: "Move or rename a document",
	Args:  cobra.ExactArgs(2),
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
		meta, err := t.GetMetadata(doc.ID)
		if err != nil {
			outputError(err)
			return err
		}

		// dest is existing folder → move; otherwise → rename
		destMatches := tree.Find(args[1])
		if len(destMatches) == 1 && destMatches[0].IsFolder() {
			meta.Parent = destMatches[0].ID
		} else {
			meta.VisibleName = args[1]
		}

		meta.MetadataModified = true
		if err := t.SetMetadata(doc.ID, meta); err != nil {
			outputError(err)
			return err
		}

		output(map[string]any{"id": doc.ID, "name": meta.VisibleName, "parent": meta.Parent, "status": "moved"})
		return nil
	},
}

func init() { rootCmd.AddCommand(mvCmd) }
