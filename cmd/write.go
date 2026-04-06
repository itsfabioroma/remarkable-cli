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
	Short: "Write text as pen strokes on a notebook page",
	Long: `Writes text onto a notebook page. Auto-scales to fit, wraps long lines.
When writing to an existing page, appends below existing content.

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
		doc, err := findDoc(sshT, args[0])
		if err != nil {
			return err
		}

		// read .content for page list
		content, rawContent, err := readContent(sshT, doc.ID)
		if err != nil {
			return err
		}
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
			return fmt.Errorf("no existing pages to use as template (need at least one page with content)")
		}

		// determine target page
		var targetPageID string
		var existingRMData []byte

		if writeNewPage {
			targetPageID = uuid.New().String()
			if err := addPageToContentAtEnd(sshT, doc.ID, rawContent, targetPageID); err != nil {
				return err
			}
			pageIDs = append(pageIDs, targetPageID)

		} else if writePage > 0 {
			idx := writePage - 1
			if idx < 0 || idx >= len(pageIDs) {
				return fmt.Errorf("page %d does not exist (notebook has %d pages)", writePage, len(pageIDs))
			}
			targetPageID = pageIDs[idx]

			// read existing page content to find bottom
			rc, err := sshT.ReadFile(doc.ID, targetPageID+".rm")
			if err == nil {
				existingRMData, _ = io.ReadAll(rc)
				rc.Close()
			}

		} else {
			if len(pageIDs) == 0 {
				return fmt.Errorf("notebook has no pages")
			}
			targetPageID = pageIDs[len(pageIDs)-1]

			rc, err := sshT.ReadFile(doc.ID, targetPageID+".rm")
			if err == nil {
				existingRMData, _ = io.ReadAll(rc)
				rc.Close()
			}
		}

		// find where to start text vertically
		startY := float32(200)
		if len(existingRMData) > 0 {
			// find the bottom of existing content
			bottomY := findBottomY(existingRMData)
			if bottomY > 100 {
				startY = bottomY + 60 // gap below existing content
			}
		}

		// render text with auto-scaling and word wrapping
		textLines, _ := render.RenderTextAutoScale(writeText, startY,
			model.PenFineliner2, model.ColorBlack, 2.0)

		if len(textLines) == 0 {
			return fmt.Errorf("text produced no strokes")
		}

		// build .rm: use template for block structure, inject our strokes
		// for existing pages, keep existing strokes and ADD ours
		var rmData []byte
		if len(existingRMData) > 0 && !writeNewPage {
			// append to existing page: keep all existing blocks + add new strokes
			rmData, err = appendStrokesToPage(existingRMData, textLines)
		} else {
			// new page: template stripped of strokes + our strokes
			rmData, err = rm.BuildPageFromTemplate(textLines, "5a7cebef-607a-495b-b747-32eb39d8c423", templateData)
		}
		if err != nil {
			return fmt.Errorf("cannot build page: %w", err)
		}

		// write to device
		rmPath := filepath.Join("/home/root/.local/share/remarkable/xochitl",
			doc.ID, targetPageID+".rm")
		sshT.RunCommand(fmt.Sprintf("mkdir -p /home/root/.local/share/remarkable/xochitl/%s", doc.ID))

		if err := sshT.WriteRawFile(rmPath, rmData); err != nil {
			return err
		}

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
			"strokes": len(textLines),
			"bytes":   len(rmData),
			"newPage": writeNewPage,
			"status":  "written",
		})
		return nil
	},
}

// findBottomY scans an .rm file and returns the lowest Y coordinate of any stroke
func findBottomY(rmData []byte) float32 {
	blocks, err := rm.ParseBlocks(rmData)
	if err != nil {
		return 0
	}

	maxY := float32(0)
	for _, b := range blocks {
		if b.Type == rm.BlockSceneLineItem {
			if ld, ok := b.Data.(*rm.SceneLineData); ok {
				for _, p := range ld.Line.Points {
					if p.Y > maxY {
						maxY = p.Y
					}
				}
			}
		}
	}
	return maxY
}

// appendStrokesToPage adds new strokes to an existing .rm file (preserving all existing content)
func appendStrokesToPage(existingData []byte, newLines []rm.Line) ([]byte, error) {
	// find parentID and max sequence from existing data
	parentID := rm.CrdtId{0, 11}
	maxSeq := uint64(100)

	blocks, _ := rm.ParseBlocks(existingData)
	for _, b := range blocks {
		if b.Type == rm.BlockSceneLineItem {
			if ld, ok := b.Data.(*rm.SceneLineData); ok {
				if !ld.ParentID.Zero() {
					parentID = ld.ParentID
				}
				if ld.ItemID.Part2 > maxSeq {
					maxSeq = ld.ItemID.Part2
				}
			}
		}
	}

	// copy entire existing file as-is
	w := rm.NewWriter()
	w.WriteBytes(existingData)

	// append new LineItem blocks at the end
	for i, line := range newLines {
		seq := maxSeq + uint64(i) + 1
		itemID := rm.CrdtId{1, seq}

		sub := rm.NewWriter()
		sub.WriteTag(1, rm.TagID)
		sub.WriteCrdtId(parentID)
		sub.WriteTag(2, rm.TagID)
		sub.WriteCrdtId(itemID)
		sub.WriteTag(3, rm.TagID)
		sub.WriteCrdtId(rm.CrdtId{})
		sub.WriteTag(4, rm.TagID)
		sub.WriteCrdtId(rm.CrdtId{})
		sub.WriteTag(5, rm.TagByte4)
		sub.WriteUint32(0)

		inner := rm.NewWriter()
		inner.WriteUint8(0x03)
		inner.WriteTag(1, rm.TagByte4)
		inner.WriteUint32(uint32(line.Tool))
		inner.WriteTag(2, rm.TagByte4)
		inner.WriteUint32(uint32(line.Color))
		inner.WriteTag(3, rm.TagByte8)
		inner.WriteFloat64(line.ThicknessScale)

		pointData := rm.NewWriter()
		pointData.WritePointsV2(line.Points)
		inner.WriteTag(5, rm.TagLength4)
		inner.WriteUint32(uint32(len(pointData.Bytes())))
		inner.WriteBytes(pointData.Bytes())

		sub.WriteTag(6, rm.TagLength4)
		sub.WriteUint32(uint32(len(inner.Bytes())))
		sub.WriteBytes(inner.Bytes())

		w.WriteUint32(uint32(len(sub.Bytes())))
		w.WriteByte(0)
		w.WriteByte(2)
		w.WriteByte(2)
		w.WriteByte(uint8(rm.BlockSceneLineItem))
		w.WriteBytes(sub.Bytes())
	}

	return w.Bytes(), nil
}

// addPageToContentAtEnd adds a new page at the END of the notebook
func addPageToContentAtEnd(sshT sshWriter, docID string, rawContent []byte, newPageID string) error {
	var raw map[string]any
	json.Unmarshal(rawContent, &raw)

	pageCount := 0
	if pc, ok := raw["pageCount"].(float64); ok {
		pageCount = int(pc)
	}

	seq := pageCount + 1
	ts := fmt.Sprintf("1:%d", seq)
	now := fmt.Sprintf("%d", time.Now().UnixMilli())

	// generate idx that sorts AFTER all existing pages
	// use "zz..." prefix which sorts after all lowercase alpha values
	lastIdx := fmt.Sprintf("zz%04d", seq)

	newPage := map[string]any{
		"id":       newPageID,
		"idx":      map[string]any{"timestamp": ts, "value": lastIdx},
		"template": map[string]any{"timestamp": ts, "value": "Blank"},
		"modifed":  now,
	}

	if cPages, ok := raw["cPages"].(map[string]any); ok {
		pages, _ := cPages["pages"].([]any)
		pages = append(pages, newPage)
		cPages["pages"] = pages
	}

	raw["pageCount"] = pageCount + 1

	newContent, _ := json.MarshalIndent(raw, "", "    ")
	contentPath := filepath.Join("/home/root/.local/share/remarkable/xochitl", docID+".content")
	return sshT.WriteRawFile(contentPath, newContent)
}

type sshWriter interface {
	RunCommand(string) (string, error)
	WriteRawFile(string, []byte) error
}

func init() {
	writeCmd.Flags().StringVar(&writeText, "text", "", "text to write (required)")
	writeCmd.Flags().IntVar(&writePage, "page", 0, "page number (1-indexed, 0=last)")
	writeCmd.Flags().BoolVar(&writeNewPage, "new-page", false, "create a new page")
	writeCmd.MarkFlagRequired("text")
	rootCmd.AddCommand(writeCmd)
}
