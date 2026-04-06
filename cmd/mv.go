package cmd

import (
	"fmt"

	"github.com/fabioroma/remarkable-cli/pkg/model"
	"github.com/fabioroma/remarkable-cli/pkg/transport"
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

		ms, ok := t.(transport.MetaStore)
		if !ok {
			err := model.NewCLIError(model.ErrUnsupported, t.Name(),
				"this transport does not support move/rename")
			outputError(err)
			return err
		}

		// find source document
		docs, err := t.ListDocuments()
		if err != nil {
			outputError(err)
			return err
		}

		tree := model.NewTree(docs)
		matches := tree.Find(args[0])
		if len(matches) == 0 {
			err := model.NewCLIError(model.ErrNotFound, t.Name(),
				fmt.Sprintf("document %q not found", args[0]))
			outputError(err)
			return err
		}
		if len(matches) > 1 {
			err := model.NewCLIError(model.ErrConflict, t.Name(),
				fmt.Sprintf("ambiguous: %d documents named %q", len(matches), args[0]))
			outputError(err)
			return err
		}

		doc := matches[0]

		// get current metadata
		meta, err := ms.GetMetadata(doc.ID)
		if err != nil {
			outputError(err)
			return err
		}

		// check if dest is an existing folder (move) or a new name (rename)
		destMatches := tree.Find(args[1])
		if len(destMatches) == 1 && destMatches[0].IsFolder() {
			// move into folder
			meta.Parent = destMatches[0].ID
		} else {
			// rename
			meta.VisibleName = args[1]
		}

		meta.MetadataModified = true
		if err := ms.SetMetadata(doc.ID, meta); err != nil {
			outputError(err)
			return err
		}

		output(map[string]any{
			"id":      doc.ID,
			"name":    meta.VisibleName,
			"parent":  meta.Parent,
			"status":  "moved",
		})

		return nil
	},
}

func init() {
	rootCmd.AddCommand(mvCmd)
}
