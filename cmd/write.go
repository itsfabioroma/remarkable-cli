package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"time"

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
	Short: "Write text on a notebook page",
	Long: `Writes text onto a notebook page by cloning a real page from the device
as template and injecting a text block. The text appears as typed text.

  remarkable write "My Notebook" --text "Meeting notes here"
  remarkable write "My Notebook" --text "Page 2 content" --page 2
  remarkable write "My Notebook" --text "New page" --new-page`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if writeText == "" {
			return fmt.Errorf("--text is required")
		}

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

		// grab a real .rm from this notebook as template
		var templateData []byte
		for _, pid := range pageIDs {
			rc, err := sshT.ReadFile(doc.ID, pid+".rm")
			if err != nil {
				continue
			}
			templateData, _ = io.ReadAll(rc)
			rc.Close()
			if len(templateData) > 100 {
				break
			}
		}

		if templateData == nil {
			return fmt.Errorf("no existing pages to use as template")
		}

		// determine target page UUID
		var targetPageID string
		if writeNewPage {
			targetPageID = uuid.New().String()
			if err := updateContentPages(sshT, doc.ID, contentData, targetPageID); err != nil {
				outputError(err)
				return err
			}
			pageIDs = append(pageIDs, targetPageID)
		} else if writePage > 0 {
			idx := writePage - 1
			if idx < 0 || idx >= len(pageIDs) {
				return fmt.Errorf("page %d does not exist (notebook has %d pages)", writePage, len(pageIDs))
			}
			targetPageID = pageIDs[idx]
		} else {
			if len(pageIDs) == 0 {
				return fmt.Errorf("notebook has no pages")
			}
			targetPageID = pageIDs[len(pageIDs)-1]
		}

		// render text as pen strokes
		// coordinates: reMarkable uses center-origin, ~-700 to +700 X, 0 to 1800 Y
		// use Fineliner v2 (17), black, thickness 2.0 to match real strokes
		textLines := render.RenderText(writeText, -500, 300, 5.0,
			model.PenFineliner2, model.ColorBlack, 2.0)

		if len(textLines) == 0 {
			return fmt.Errorf("text produced no strokes")
		}

		// fix point widths to match real device strokes (width=16, pressure range 8-30)
		for i := range textLines {
			for j := range textLines[i].Points {
				textLines[i].Points[j].Width = 16
				textLines[i].Points[j].Pressure = 20
				textLines[i].Points[j].Speed = 0
			}
		}

		// strip template strokes, inject ours using correct parentID
		// build .rm using real template (preserves all block structure, uses correct parentID)
		rmData, err := rm.BuildPageFromTemplate(textLines, "5a7cebef-607a-495b-b747-32eb39d8c423", templateData)
		if err != nil {
			return fmt.Errorf("cannot build page: %w", err)
		}

		// write to device
		rmPath := filepath.Join("/home/root/.local/share/remarkable/xochitl",
			doc.ID, targetPageID+".rm")
		sshT.RunCommand(fmt.Sprintf("mkdir -p /home/root/.local/share/remarkable/xochitl/%s", doc.ID))

		if err := sshT.WriteRawFile(rmPath, rmData); err != nil {
			outputError(err)
			return err
		}

		// restart xochitl
		sshT.RunCommand("systemctl restart xochitl")

		pageNum := len(pageIDs)
		if writePage > 0 {
			pageNum = writePage
		}

		output(map[string]any{
			"id":      doc.ID,
			"name":    doc.Name,
			"pageId":  targetPageID,
			"page":    pageNum,
			"text":    writeText,
			"bytes":   len(rmData),
			"newPage": writeNewPage,
			"status":  "written",
		})
		return nil
	},
}


type sshWriter interface {
	RunCommand(string) (string, error)
	WriteRawFile(string, []byte) error
}

func updateContentPages(sshT sshWriter, docID string, originalContent []byte, newPageID string) error {
	var raw map[string]any
	json.Unmarshal(originalContent, &raw)

	pageCount := 0
	if pc, ok := raw["pageCount"].(float64); ok {
		pageCount = int(pc)
	}
	seq := pageCount + 1
	timestamp := fmt.Sprintf("1:%d", seq)
	now := fmt.Sprintf("%d", time.Now().UnixMilli())

	lastIdx := "ba"
	if cPages, ok := raw["cPages"].(map[string]any); ok {
		pages, _ := cPages["pages"].([]any)
		if len(pages) > 0 {
			lastPage, _ := pages[len(pages)-1].(map[string]any)
			if idx, ok := lastPage["idx"].(map[string]any); ok {
				if v, ok := idx["value"].(string); ok {
					lastIdx = v + "a"
				}
			}
		}
	}

	newPage := map[string]any{
		"id":      newPageID,
		"idx":     map[string]any{"timestamp": timestamp, "value": lastIdx},
		"template": map[string]any{"timestamp": timestamp, "value": "Blank"},
		"modifed": now,
	}

	if cPages, ok := raw["cPages"].(map[string]any); ok {
		pages, _ := cPages["pages"].([]any)
		pages = append(pages, newPage)
		cPages["pages"] = pages
		raw["cPages"] = cPages
	} else {
		pages, _ := raw["pages"].([]any)
		pages = append(pages, newPageID)
		raw["pages"] = pages
	}

	raw["pageCount"] = pageCount + 1

	newContent, _ := json.MarshalIndent(raw, "", "    ")
	contentPath := filepath.Join("/home/root/.local/share/remarkable/xochitl", docID+".content")
	return sshT.WriteRawFile(contentPath, newContent)
}

func init() {
	writeCmd.Flags().StringVar(&writeText, "text", "", "text to write (required)")
	writeCmd.Flags().IntVar(&writePage, "page", 0, "page number (1-indexed, 0=last)")
	writeCmd.Flags().BoolVar(&writeNewPage, "new-page", false, "create a new page")
	writeCmd.MarkFlagRequired("text")
	rootCmd.AddCommand(writeCmd)
}
