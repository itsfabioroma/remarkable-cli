package cmd

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/itsfabioroma/remarkable-cli/pkg/encoding/rm"
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
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

		// build the .rm file: clone template, strip strokes + old text, inject our text
		rmData, err := buildTextPage(templateData, writeText)
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

// buildTextPage clones a real .rm template, strips line/text blocks, injects new text
func buildTextPage(template []byte, text string) ([]byte, error) {
	if len(template) < 50 {
		return nil, fmt.Errorf("template too short")
	}

	w := rm.NewWriter()
	w.WriteHeader()

	// parse template blocks — keep envelope, strip strokes and old text
	pos := len(rm.V6Header)
	for pos+8 < len(template) {
		blockLen := binary.LittleEndian.Uint32(template[pos : pos+4])
		payloadEnd := pos + 8 + int(blockLen)
		if payloadEnd > len(template) {
			break
		}

		bt := template[pos+7]

		// skip LineItem and RootText blocks (we'll add our own text)
		if rm.BlockType(bt) == rm.BlockSceneLineItem || rm.BlockType(bt) == rm.BlockRootText {
			pos = payloadEnd
			continue
		}

		// copy block as-is
		w.WriteBytes(template[pos:payloadEnd])
		pos = payloadEnd
	}

	// inject our RootText block with the text content
	writeRootTextBlock(w, text)

	return w.Bytes(), nil
}

// writeRootTextBlock creates a RootText block with plain text content
// format reverse-engineered from real device .rm files
func writeRootTextBlock(w *rm.Writer, text string) {
	// build the text content sub-block
	// structure: CRDT sequence with one item containing the full text
	textBytes := []byte(text)

	// inner-most: the text string with length prefix
	// format: varuint(len) + 0x01 (is_ascii flag) + bytes
	textPayload := rm.NewWriter()
	textPayload.WriteVaruint(uint64(len(textBytes)))
	textPayload.WriteByte(0x01) // is_ascii
	textPayload.WriteBytes(textBytes)

	// CRDT item: item_id, left_id, right_id, deleted, value
	crdtItem := rm.NewWriter()
	crdtItem.WriteTag(2, rm.TagID)
	crdtItem.WriteCrdtId(rm.CrdtId{1, 15}) // item_id
	crdtItem.WriteTag(3, rm.TagID)
	crdtItem.WriteCrdtId(rm.CrdtId{0, 0}) // left_id
	crdtItem.WriteTag(4, rm.TagID)
	crdtItem.WriteCrdtId(rm.CrdtId{0, 0}) // right_id
	crdtItem.WriteTag(5, rm.TagByte4)
	crdtItem.WriteUint32(0) // deleted_length
	crdtItem.WriteTag(6, rm.TagLength4)
	crdtItem.WriteUint32(uint32(len(textPayload.Bytes())))
	crdtItem.WriteBytes(textPayload.Bytes())

	// wrap in item container: tag(0, Length4) + content
	itemContainer := rm.NewWriter()
	itemContainer.WriteTag(0, rm.TagLength4)
	itemContainer.WriteUint32(uint32(len(crdtItem.Bytes())))
	itemContainer.WriteBytes(crdtItem.Bytes())

	// item list: varuint count + items
	itemList := rm.NewWriter()
	itemList.WriteVaruint(1) // 1 item
	itemList.WriteBytes(itemContainer.Bytes())

	// wrap in tag(1, Length4) — text items
	textItems := rm.NewWriter()
	textItems.WriteTag(1, rm.TagLength4)
	textItems.WriteUint32(uint32(len(itemList.Bytes())))
	textItems.WriteBytes(itemList.Bytes())

	// paragraph styles: tag(2, Length4) — empty for plain text
	paraStyles := rm.NewWriter()
	paraStyles.WriteTag(1, rm.TagLength4)
	paraStyles.WriteUint32(0) // empty

	// text content wrapper: tag(1) items + tag(2) styles
	contentWrapper := rm.NewWriter()
	contentWrapper.WriteTag(1, rm.TagLength4)
	contentWrapper.WriteUint32(uint32(len(textItems.Bytes())))
	contentWrapper.WriteBytes(textItems.Bytes())
	contentWrapper.WriteTag(2, rm.TagLength4)
	contentWrapper.WriteUint32(uint32(len(paraStyles.Bytes())))
	contentWrapper.WriteBytes(paraStyles.Bytes())

	// full RootText payload: tag(1) block_id + tag(2) content + tag(3) position + tag(4) width
	payload := rm.NewWriter()

	// tag(1, TagID) block_id = {0, 0}
	payload.WriteTag(1, rm.TagID)
	payload.WriteCrdtId(rm.CrdtId{0, 0})

	// tag(2, Length4) content
	payload.WriteTag(2, rm.TagLength4)
	payload.WriteUint32(uint32(len(contentWrapper.Bytes())))
	payload.WriteBytes(contentWrapper.Bytes())

	// tag(3, Length4) position: float64 x, float64 y
	posData := rm.NewWriter()
	posData.WriteFloat64(100.0) // x
	posData.WriteFloat64(200.0) // y
	payload.WriteTag(3, rm.TagLength4)
	payload.WriteUint32(uint32(len(posData.Bytes())))
	payload.WriteBytes(posData.Bytes())

	// tag(4, Byte4) width
	payload.WriteTag(4, rm.TagByte4)
	payload.WriteUint32(936) // ~width in device units (matches real files)

	// write block envelope: min=0, cur=1
	w.WriteUint32(uint32(len(payload.Bytes())))
	w.WriteByte(0)
	w.WriteByte(0) // min_version
	w.WriteByte(1) // cur_version
	w.WriteByte(uint8(rm.BlockRootText))
	w.WriteBytes(payload.Bytes())
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
