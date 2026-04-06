package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/fabioroma/remarkable-cli/pkg/encoding/rm"
	"github.com/fabioroma/remarkable-cli/pkg/model"
	"github.com/fabioroma/remarkable-cli/pkg/render"
	"github.com/fabioroma/remarkable-cli/pkg/transport"
	"github.com/spf13/cobra"
)

var (
	exportFormat string
	exportOutput string
)

var exportCmd = &cobra.Command{
	Use:   "export <name>",
	Short: "Render document annotations to SVG/PDF/PNG",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		// need full transport for raw .rm files + content metadata
		fullT, ok := t.(transport.FullTransport)
		if !ok {
			err := model.NewCLIError(model.ErrUnsupported, t.Name(),
				"export requires SSH or Cloud transport (USB only supports PDF download)")
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

		// read .content to get page UUIDs
		pageIDs, err := readPageIDs(fullT, doc.ID)
		if err != nil {
			outputError(err)
			return err
		}

		if len(pageIDs) == 0 {
			err := model.NewCLIError(model.ErrNotFound, t.Name(),
				fmt.Sprintf("no pages found for %q", doc.Name))
			outputError(err)
			return err
		}

		// determine output directory
		outDir := exportOutput
		if outDir == "" {
			outDir = doc.Name + "_export"
		}
		os.MkdirAll(outDir, 0755)

		// render each page
		renderer := render.NewSVGRenderer()
		var exported []map[string]any

		for i, pageID := range pageIDs {
			// read the .rm file
			rc, err := fullT.ReadFile(doc.ID, pageID+".rm")
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: cannot read page %d: %v\n", i+1, err)
				continue
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}

			// parse blocks
			blocks, err := rm.ParseBlocks(data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: parse error on page %d: %v\n", i+1, err)
				continue
			}

			// render to SVG
			outFile := filepath.Join(outDir, fmt.Sprintf("page_%03d.svg", i+1))
			f, err := os.Create(outFile)
			if err != nil {
				return err
			}

			if err := renderer.RenderPage(f, blocks); err != nil {
				f.Close()
				continue
			}
			f.Close()

			exported = append(exported, map[string]any{
				"page": i + 1,
				"file": outFile,
			})
		}

		result := map[string]any{
			"id":     doc.ID,
			"name":   doc.Name,
			"format": exportFormat,
			"pages":  exported,
			"output": outDir,
		}
		output(result)

		if !flagJSON && isTerminal() {
			fmt.Printf("Exported %d pages to %s/\n", len(exported), outDir)
		}

		return nil
	},
}

// readPageIDs reads page UUIDs from a document's .content file
func readPageIDs(t transport.FullTransport, docID string) ([]string, error) {
	rc, err := t.ReadFile(docID, "content")
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var content model.Content
	if err := json.NewDecoder(rc).Decode(&content); err != nil {
		return nil, fmt.Errorf("invalid .content file: %w", err)
	}

	return content.PageIDs(), nil
}

func init() {
	exportCmd.Flags().StringVarP(&exportFormat, "format", "f", "svg", "output format: svg, pdf, png")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "output directory")
	rootCmd.AddCommand(exportCmd)
}
