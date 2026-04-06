package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/itsfabioroma/remarkable-cli/pkg/encoding/rm"
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/itsfabioroma/remarkable-cli/pkg/render"
	"github.com/spf13/cobra"
)

var exportOutput string

var exportCmd = &cobra.Command{
	Use:   "export <name>",
	Short: "Render document annotations to SVG",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		// find document
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

		// read .content for page UUIDs
		rc, err := t.ReadFile(doc.ID, "content")
		if err != nil {
			outputError(err)
			return err
		}
		var content model.Content
		json.NewDecoder(rc).Decode(&content)
		rc.Close()

		pageIDs := content.PageIDs()
		if len(pageIDs) == 0 {
			err := model.NewCLIError(model.ErrNotFound, t.Name(), fmt.Sprintf("no pages in %q", doc.Name))
			outputError(err)
			return err
		}

		// output dir
		outDir := exportOutput
		if outDir == "" {
			outDir = doc.Name + "_export"
		}
		os.MkdirAll(outDir, 0755)

		// render each page
		renderer := render.NewSVGRenderer()
		var exported []map[string]any

		for i, pageID := range pageIDs {
			rc, err := t.ReadFile(doc.ID, pageID+".rm")
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(rc)
			rc.Close()

			blocks, err := rm.ParseBlocks(data)
			if err != nil {
				continue
			}

			outFile := filepath.Join(outDir, fmt.Sprintf("page_%03d.svg", i+1))
			f, err := os.Create(outFile)
			if err != nil {
				continue
			}
			renderer.RenderPage(f, blocks)
			f.Close()

			exported = append(exported, map[string]any{"page": i + 1, "file": outFile})
		}

		output(map[string]any{"id": doc.ID, "name": doc.Name, "pages": exported, "output": outDir})
		return nil
	},
}

func init() {
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "output directory")
	rootCmd.AddCommand(exportCmd)
}
