package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/fabioroma/remarkable-cli/pkg/model"
	"github.com/fabioroma/remarkable-cli/pkg/transport"
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

		// check transport supports reading
		reader, ok := t.(transport.Reader)
		if !ok {
			err := model.NewCLIError(model.ErrUnsupported, t.Name(),
				"this transport does not support file downloads")
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

		// determine file extension
		ext := ".pdf"
		if doc.FileType == "epub" {
			ext = ".epub"
		}

		// download the source file
		rc, err := reader.ReadFile(doc.ID, doc.FileType)
		if err != nil {
			outputError(err)
			return err
		}
		defer rc.Close()

		// write to local file
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

		// output result
		output(map[string]any{
			"id":       doc.ID,
			"name":     doc.Name,
			"path":     outPath,
			"size":     n,
			"fileType": doc.FileType,
		})

		return nil
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}
