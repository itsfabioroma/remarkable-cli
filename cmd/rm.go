package cmd

import (
	"fmt"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Delete a document or folder",
	Long:  `Delete a document or folder. Resolves by name; errors if ambiguous.`,
	Example: `  remarkable rm "Old Draft"
  remarkable rm "Archive"`,
	Args: cobra.ExactArgs(1),
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

		if err := t.DeleteDocument(matches[0].ID); err != nil {
			outputError(err)
			return err
		}

		output(map[string]any{"id": matches[0].ID, "name": matches[0].Name, "status": "deleted"})
		return nil
	},
}

func init() { rootCmd.AddCommand(rmCmd) }
