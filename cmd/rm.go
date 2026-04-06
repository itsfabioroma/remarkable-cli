package cmd

import (
	"fmt"

	"github.com/fabioroma/remarkable-cli/pkg/model"
	"github.com/fabioroma/remarkable-cli/pkg/transport"
	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Delete a document or folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		deleter, ok := t.(transport.Deleter)
		if !ok {
			err := model.NewCLIError(model.ErrUnsupported, t.Name(),
				"this transport does not support deletion")
			outputError(err)
			return err
		}

		// find the document
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
		if err := deleter.DeleteDocument(doc.ID); err != nil {
			outputError(err)
			return err
		}

		output(map[string]any{
			"id":     doc.ID,
			"name":   doc.Name,
			"status": "deleted",
		})

		return nil
	},
}

func init() {
	rootCmd.AddCommand(rmCmd)
}
