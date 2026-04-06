package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/itsfabioroma/remarkable-cli/pkg/encoding/rm"
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/itsfabioroma/remarkable-cli/pkg/render"
	"github.com/spf13/cobra"
)

var (
	writeText    string
	writePage    int
	writeNewPage bool
)

var writeCmd = &cobra.Command{
	Use:   "write <notebook>",
	Short: "Write text as handwriting on a notebook page",
	Long: `Converts text into pen strokes and writes them to a notebook page.
The text appears as handwritten fineliner strokes on the device.

  remarkable write "My Notebook" --text "Meeting notes here"
  remarkable write "My Notebook" --text "Page 2 content" --page 2
  remarkable write "My Notebook" --text "New page" --new-page`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if writeText == "" {
			return fmt.Errorf("--text is required")
		}

		// need SSH for filesystem access
		sshT, err := getSSH()
		if err != nil {
			return err
		}
		defer sshT.Close()

		// find the notebook
		docs, err := sshT.ListDocuments()
		if err != nil {
			outputError(err)
			return err
		}

		tree := model.NewTree(docs)
		matches := tree.Find(args[0])
		if len(matches) == 0 {
			err := model.NewCLIError(model.ErrNotFound, "ssh", fmt.Sprintf("%q not found", args[0]))
			outputError(err)
			return err
		}
		if len(matches) > 1 {
			err := model.NewCLIError(model.ErrConflict, "ssh", fmt.Sprintf("ambiguous: %d docs named %q", len(matches), args[0]))
			outputError(err)
			return err
		}

		doc := matches[0]

		// read .content for page list
		rc, err := sshT.ReadFile(doc.ID, "content")
		if err != nil {
			outputError(err)
			return err
		}
		contentData, _ := io.ReadAll(rc)
		rc.Close()

		var content model.Content
		json.Unmarshal(contentData, &content)
		pageIDs := content.PageIDs()

		// determine target page UUID
		var targetPageID string

		if writeNewPage {
			// create a new page
			targetPageID = uuid.New().String()
			pageIDs = append(pageIDs, targetPageID)

			// update .content with new page
			if err := updateContentPages(sshT, doc.ID, contentData, targetPageID); err != nil {
				outputError(err)
				return err
			}

		} else if writePage > 0 {
			// specific page number (1-indexed)
			idx := writePage - 1
			if idx < 0 || idx >= len(pageIDs) {
				err := model.NewCLIError(model.ErrNotFound, "ssh",
					fmt.Sprintf("page %d does not exist (notebook has %d pages)", writePage, len(pageIDs)))
				outputError(err)
				return err
			}
			targetPageID = pageIDs[idx]

		} else {
			// default: last page
			if len(pageIDs) == 0 {
				err := model.NewCLIError(model.ErrNotFound, "ssh", "notebook has no pages")
				outputError(err)
				return err
			}
			targetPageID = pageIDs[len(pageIDs)-1]
		}

		// convert text to pen strokes
		lines := render.RenderText(writeText, 100, 200, 3.0,
			model.PenFineliner, model.ColorBlack, 1.5)

		if len(lines) == 0 {
			return fmt.Errorf("text produced no strokes")
		}

		// build the .rm v6 file
		rmData, err := rm.BuildPage(lines, "00000000-0000-0000-0000-000000000000")
		if err != nil {
			return fmt.Errorf("cannot build .rm page: %w", err)
		}

		// write the .rm file to device
		rmPath := filepath.Join("/home/root/.local/share/remarkable/xochitl",
			doc.ID, targetPageID+".rm")

		// ensure the doc directory exists
		sshT.RunCommand(fmt.Sprintf("mkdir -p /home/root/.local/share/remarkable/xochitl/%s", doc.ID))

		if err := sshT.WriteRawFile(rmPath, rmData); err != nil {
			outputError(err)
			return err
		}

		pageNum := len(pageIDs)
		if writePage > 0 {
			pageNum = writePage
		}

		output(map[string]any{
			"id":       doc.ID,
			"name":     doc.Name,
			"pageId":   targetPageID,
			"page":     pageNum,
			"strokes":  len(lines),
			"bytes":    len(rmData),
			"newPage":  writeNewPage,
			"status":   "written",
		})

		if isTerminal() {
			fmt.Printf("Wrote %d strokes to page %d of %q (%d bytes)\n",
				len(lines), pageNum, doc.Name, len(rmData))
		}

		return nil
	},
}

// updateContentPages adds a new page UUID to the .content file
func updateContentPages(sshT interface{ RunCommand(string) (string, error) }, docID string, originalContent []byte, newPageID string) error {
	// parse the original content
	var raw map[string]any
	json.Unmarshal(originalContent, &raw)

	// check if using cPages (new format) or pages (old format)
	if cPages, ok := raw["cPages"].(map[string]any); ok {
		// new format: add to cPages.pages array
		pages, _ := cPages["pages"].([]any)
		pages = append(pages, map[string]any{"id": newPageID})
		cPages["pages"] = pages
		raw["cPages"] = cPages
	} else {
		// old format: add to pages array
		pages, _ := raw["pages"].([]any)
		pages = append(pages, newPageID)
		raw["pages"] = pages
	}

	// update page count
	if pc, ok := raw["pageCount"].(float64); ok {
		raw["pageCount"] = pc + 1
	}

	// write back
	newContent, _ := json.MarshalIndent(raw, "", "    ")
	contentPath := filepath.Join("/home/root/.local/share/remarkable/xochitl", docID+".content")

	escaped := strings.ReplaceAll(string(newContent), "'", "'\\''")
	_, err := sshT.RunCommand(fmt.Sprintf("printf '%%s' '%s' > %s", escaped, contentPath))
	if err != nil {
		// fallback: write via temp file
		tmpPath := "/tmp/remarkable-content-" + docID + ".json"
		os.WriteFile(tmpPath, newContent, 0644)
		return fmt.Errorf("cannot update .content: %w", err)
	}

	return nil
}

func init() {
	writeCmd.Flags().StringVar(&writeText, "text", "", "text to write as handwriting (required)")
	writeCmd.Flags().IntVar(&writePage, "page", 0, "page number to write on (1-indexed, 0=last page)")
	writeCmd.Flags().BoolVar(&writeNewPage, "new-page", false, "create a new page for the text")
	writeCmd.MarkFlagRequired("text")
	rootCmd.AddCommand(writeCmd)
}
