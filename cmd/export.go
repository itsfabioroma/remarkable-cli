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

var (
	exportOutput string
	exportPage   int
	exportSVG    bool // opt-in for SVG-only (default is PNG)
)

var exportCmd = &cobra.Command{
	Use:   "export <name>",
	Short: "Export notebook pages to SVG or PNG",
	Long: `Export handwritten pages as PNG (default) or SVG.

Uses SSH when available, falls back to cloud automatically. Renders strokes from the v6 .rm format.`,
	Example: `  remarkable export "Notebook" -o /tmp
  remarkable export "Notebook" --svg
  remarkable export "Notebook" --page 3`,
	Args: cobra.ExactArgs(1),
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

		// filter to single page if --page is set
		if exportPage > 0 {
			idx := exportPage - 1
			if idx < 0 || idx >= len(pageIDs) {
				return fmt.Errorf("page %d does not exist (notebook has %d pages)", exportPage, len(pageIDs))
			}
			pageIDs = []string{pageIDs[idx]}
		}

		// output dir
		outDir := exportOutput
		if outDir == "" {
			outDir = doc.Name + "_export"
		}
		// create output dir → wrap in CLIError envelope
		if err := os.MkdirAll(outDir, 0755); err != nil {
			e := model.NewCLIError(model.ErrIO, "", fmt.Sprintf("cannot create %s: %v", outDir, err))
			outputError(e)
			return e
		}

		// download source file for PDF/EPUB documents
		var sourceFile string
		if doc.FileType == "pdf" || doc.FileType == "epub" {
			rc, err := t.ReadFile(doc.ID, doc.FileType)
			if err == nil {
				sourceData, _ := io.ReadAll(rc)
				rc.Close()
				sourceFile = filepath.Join(outDir, doc.Name+"."+doc.FileType)
				// write source file → wrap in CLIError envelope
				if err := os.WriteFile(sourceFile, sourceData, 0644); err != nil {
					e := model.NewCLIError(model.ErrIO, "", fmt.Sprintf("cannot write %s: %v", sourceFile, err))
					outputError(e)
					return e
				}
			}
		}

		// render each page
		renderer := render.NewSVGRenderer()
		var exported []map[string]any

		for i, pageID := range pageIDs {
			pageNum := i + 1
			if exportPage > 0 {
				pageNum = exportPage
			}

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

			// render SVG
			svgFile := filepath.Join(outDir, fmt.Sprintf("page_%03d.svg", pageNum))
			f, err := os.Create(svgFile)
			if err != nil {
				continue
			}
			renderer.RenderPage(f, blocks)
			f.Close()

			entry := map[string]any{"page": pageNum}

			if exportSVG {
				entry["file"] = svgFile
			} else {
				// render PNG directly in Go — no external tools needed
				pngFile := filepath.Join(outDir, fmt.Sprintf("page_%03d.png", pageNum))
				pf, err := os.Create(pngFile)
				if err == nil {
					render.RenderPagePNG(pf, blocks)
					pf.Close()
					entry["file"] = pngFile
					os.Remove(svgFile) // clean up SVG
				} else {
					entry["file"] = svgFile
				}
			}

			exported = append(exported, entry)
		}

		// build result
		result := map[string]any{"id": doc.ID, "name": doc.Name, "pages": exported, "output": outDir}
		if sourceFile != "" {
			result["source_file"] = sourceFile
		}
		output(result)
		return nil
	},
}


func init() {
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "output directory")
	exportCmd.Flags().IntVar(&exportPage, "page", 0, "export single page (1-indexed)")
	exportCmd.Flags().BoolVar(&exportSVG, "svg", false, "output SVG instead of PNG")
	rootCmd.AddCommand(exportCmd)
}
