package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Download a document from the device",
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

		// determine file type — default to pdf
		fileType := doc.FileType
		if fileType == "" {
			fileType = "pdf"
		}

		ext := "." + fileType
		rc, err := t.ReadFile(doc.ID, fileType)
		if err != nil {
			outputError(err)
			return err
		}
		defer rc.Close()

		outPath := doc.Name + ext
		f, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer f.Close()

		n, err := io.Copy(f, rc)
		if err != nil {
			return err
		}

		output(map[string]any{"id": doc.ID, "name": doc.Name, "path": outPath, "size": n})
		return nil
	},
}

func init() { rootCmd.AddCommand(getCmd) }
