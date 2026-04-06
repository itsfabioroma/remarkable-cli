package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/spf13/cobra"
)

var pagesCmd = &cobra.Command{
	Use:   "pages <notebook>",
	Short: "List pages in a notebook",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sshT, err := getSSH()
		if err != nil {
			return err
		}
		defer sshT.Close()

		doc, err := findDoc(sshT, args[0])
		if err != nil {
			return err
		}

		content, _, err := readContent(sshT, doc.ID)
		if err != nil {
			return err
		}

		pages := content.GetCPages()
		var result []map[string]any
		for i, p := range pages {
			entry := map[string]any{
				"page":     i + 1,
				"id":       p.ID,
				"template": p.Template(),
			}
			result = append(result, entry)
		}

		output(result)
		return nil
	},
}

var pagesAddCmd = &cobra.Command{
	Use:   "add <notebook>",
	Short: "Add a blank page to a notebook",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sshT, err := getSSH()
		if err != nil {
			return err
		}
		defer sshT.Close()

		doc, err := findDoc(sshT, args[0])
		if err != nil {
			return err
		}

		_, rawContent, err := readContent(sshT, doc.ID)
		if err != nil {
			return err
		}

		tmpl, _ := cmd.Flags().GetString("template")
		afterPage, _ := cmd.Flags().GetInt("after")

		newPageID := uuid.New().String()
		if err := addPageToContent(sshT, doc.ID, rawContent, newPageID, tmpl, afterPage); err != nil {
			return err
		}

		sshT.RunCommand("systemctl restart xochitl")

		output(map[string]any{
			"id":       newPageID,
			"notebook": doc.Name,
			"template": tmpl,
			"status":   "added",
		})
		return nil
	},
}

var pagesRmCmd = &cobra.Command{
	Use:   "rm <notebook> --page <n>",
	Short: "Remove a page from a notebook",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pageNum, _ := cmd.Flags().GetInt("page")
		if pageNum < 1 {
			return fmt.Errorf("--page is required (1-indexed)")
		}

		sshT, err := getSSH()
		if err != nil {
			return err
		}
		defer sshT.Close()

		doc, err := findDoc(sshT, args[0])
		if err != nil {
			return err
		}

		content, rawContent, err := readContent(sshT, doc.ID)
		if err != nil {
			return err
		}

		pages := content.GetCPages()
		idx := pageNum - 1
		if idx < 0 || idx >= len(pages) {
			return fmt.Errorf("page %d does not exist (notebook has %d pages)", pageNum, len(pages))
		}

		removedID := pages[idx].ID

		// remove page from cPages
		if err := removePageFromContent(sshT, doc.ID, rawContent, idx); err != nil {
			return err
		}

		// delete the .rm file
		rmPath := filepath.Join("/home/root/.local/share/remarkable/xochitl", doc.ID, removedID+".rm")
		sshT.RunCommand(fmt.Sprintf("rm -f %s", rmPath))

		sshT.RunCommand("systemctl restart xochitl")

		output(map[string]any{
			"page":     pageNum,
			"id":       removedID,
			"notebook": doc.Name,
			"status":   "removed",
		})
		return nil
	},
}

var pagesMoveCmd = &cobra.Command{
	Use:   "move <notebook> --page <n> --to <m>",
	Short: "Move a page to a different position",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fromPage, _ := cmd.Flags().GetInt("page")
		toPage, _ := cmd.Flags().GetInt("to")
		if fromPage < 1 || toPage < 1 {
			return fmt.Errorf("--page and --to are required (1-indexed)")
		}

		sshT, err := getSSH()
		if err != nil {
			return err
		}
		defer sshT.Close()

		doc, err := findDoc(sshT, args[0])
		if err != nil {
			return err
		}

		_, rawContent, err := readContent(sshT, doc.ID)
		if err != nil {
			return err
		}

		if err := reorderPageInContent(sshT, doc.ID, rawContent, fromPage-1, toPage-1); err != nil {
			return err
		}

		sshT.RunCommand("systemctl restart xochitl")

		output(map[string]any{
			"notebook": doc.Name,
			"from":     fromPage,
			"to":       toPage,
			"status":   "moved",
		})
		return nil
	},
}

// --- helpers ---

func findDoc(t interface{ ListDocuments() ([]model.Document, error) }, name string) (*model.Document, error) {
	docs, err := t.ListDocuments()
	if err != nil {
		return nil, err
	}
	tree := model.NewTree(docs)
	matches := tree.Find(name)
	if len(matches) == 0 {
		return nil, model.NewCLIError(model.ErrNotFound, "", fmt.Sprintf("%q not found", name))
	}
	if len(matches) > 1 {
		return nil, model.NewCLIError(model.ErrConflict, "", fmt.Sprintf("ambiguous: %d docs named %q", len(matches), name))
	}
	return matches[0], nil
}

func readContent(sshT interface{ ReadFile(string, string) (io.ReadCloser, error) }, docID string) (*model.Content, []byte, error) {
	rc, err := sshT.ReadFile(docID, "content")
	if err != nil {
		return nil, nil, err
	}
	raw, _ := io.ReadAll(rc)
	rc.Close()

	var content model.Content
	json.Unmarshal(raw, &content)
	return &content, raw, nil
}

func addPageToContent(sshT sshWriter, docID string, rawContent []byte, newPageID, template string, afterPage int) error {
	var raw map[string]any
	json.Unmarshal(rawContent, &raw)

	if template == "" {
		template = "Blank"
	}

	pageCount := 0
	if pc, ok := raw["pageCount"].(float64); ok {
		pageCount = int(pc)
	}

	seq := pageCount + 1
	ts := fmt.Sprintf("1:%d", seq)

	newPage := map[string]any{
		"id":       newPageID,
		"idx":      map[string]any{"timestamp": ts, "value": fmt.Sprintf("p%d", seq)},
		"template": map[string]any{"timestamp": ts, "value": template},
		"modifed":  fmt.Sprintf("%d", time.Now().UnixMilli()),
	}

	if cPages, ok := raw["cPages"].(map[string]any); ok {
		pages, _ := cPages["pages"].([]any)
		if afterPage > 0 && afterPage <= len(pages) {
			// insert after specific page
			newPages := make([]any, 0, len(pages)+1)
			newPages = append(newPages, pages[:afterPage]...)
			newPages = append(newPages, newPage)
			newPages = append(newPages, pages[afterPage:]...)
			pages = newPages
		} else {
			pages = append(pages, newPage)
		}
		cPages["pages"] = pages
	}

	raw["pageCount"] = pageCount + 1

	newContent, _ := json.MarshalIndent(raw, "", "    ")
	contentPath := filepath.Join("/home/root/.local/share/remarkable/xochitl", docID+".content")
	return sshT.WriteRawFile(contentPath, newContent)
}

func removePageFromContent(sshT sshWriter, docID string, rawContent []byte, pageIdx int) error {
	var raw map[string]any
	json.Unmarshal(rawContent, &raw)

	if cPages, ok := raw["cPages"].(map[string]any); ok {
		pages, _ := cPages["pages"].([]any)
		if pageIdx >= 0 && pageIdx < len(pages) {
			pages = append(pages[:pageIdx], pages[pageIdx+1:]...)
			cPages["pages"] = pages
		}
	}

	if pc, ok := raw["pageCount"].(float64); ok && pc > 0 {
		raw["pageCount"] = pc - 1
	}

	newContent, _ := json.MarshalIndent(raw, "", "    ")
	contentPath := filepath.Join("/home/root/.local/share/remarkable/xochitl", docID+".content")
	return sshT.WriteRawFile(contentPath, newContent)
}

func reorderPageInContent(sshT sshWriter, docID string, rawContent []byte, fromIdx, toIdx int) error {
	var raw map[string]any
	json.Unmarshal(rawContent, &raw)

	if cPages, ok := raw["cPages"].(map[string]any); ok {
		pages, _ := cPages["pages"].([]any)
		if fromIdx < 0 || fromIdx >= len(pages) || toIdx < 0 || toIdx >= len(pages) {
			return fmt.Errorf("page index out of range")
		}

		// remove from old position
		page := pages[fromIdx]
		pages = append(pages[:fromIdx], pages[fromIdx+1:]...)

		// insert at new position
		newPages := make([]any, 0, len(pages)+1)
		newPages = append(newPages, pages[:toIdx]...)
		newPages = append(newPages, page)
		newPages = append(newPages, pages[toIdx:]...)

		cPages["pages"] = newPages
	}

	newContent, _ := json.MarshalIndent(raw, "", "    ")
	contentPath := filepath.Join("/home/root/.local/share/remarkable/xochitl", docID+".content")
	return sshT.WriteRawFile(contentPath, newContent)
}

// copy a page from one notebook to another
var pagesCopyCmd = &cobra.Command{
	Use:   "copy <notebook> --page <n> --to <destination>",
	Short: "Copy a page from one notebook to another",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pageNum, _ := cmd.Flags().GetInt("page")
		destName, _ := cmd.Flags().GetString("to")
		if pageNum < 1 {
			return fmt.Errorf("--page is required (1-indexed)")
		}
		if destName == "" {
			return fmt.Errorf("--to is required (destination notebook)")
		}

		sshT, err := getSSH()
		if err != nil {
			return err
		}
		defer sshT.Close()

		// resolve source and destination notebooks
		srcDoc, err := findDoc(sshT, args[0])
		if err != nil {
			return err
		}
		dstDoc, err := findDoc(sshT, destName)
		if err != nil {
			return err
		}

		// read source content to get page UUID
		srcContent, _, err := readContent(sshT, srcDoc.ID)
		if err != nil {
			return err
		}
		srcPages := srcContent.GetCPages()
		idx := pageNum - 1
		if idx < 0 || idx >= len(srcPages) {
			return fmt.Errorf("page %d does not exist (source has %d pages)", pageNum, len(srcPages))
		}
		srcPageID := srcPages[idx].ID

		// read destination content for appending
		_, dstRawContent, err := readContent(sshT, dstDoc.ID)
		if err != nil {
			return err
		}

		// copy .rm file with a new UUID
		newPageID := uuid.New().String()
		srcRmPath := filepath.Join(xochitlPath, srcDoc.ID, srcPageID+".rm")
		dstRmPath := filepath.Join(xochitlPath, dstDoc.ID, newPageID+".rm")
		if _, err := sshT.RunCommand(fmt.Sprintf("cp %s %s", srcRmPath, dstRmPath)); err != nil {
			return fmt.Errorf("failed to copy .rm file: %w", err)
		}

		// add page to destination .content
		if err := addPageToContentAtEnd(sshT, dstDoc.ID, dstRawContent, newPageID); err != nil {
			return err
		}

		sshT.RunCommand("systemctl restart xochitl")

		output(map[string]any{
			"source":      srcDoc.Name,
			"destination": dstDoc.Name,
			"sourcePage":  pageNum,
			"newPageId":   newPageID,
			"status":      "copied",
		})
		return nil
	},
}

// move a page from one notebook to another (copy + delete from source)
var pagesMoveToCmd = &cobra.Command{
	Use:   "move-to <notebook> --page <n> --to <destination>",
	Short: "Move a page to another notebook",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pageNum, _ := cmd.Flags().GetInt("page")
		destName, _ := cmd.Flags().GetString("to")
		if pageNum < 1 {
			return fmt.Errorf("--page is required (1-indexed)")
		}
		if destName == "" {
			return fmt.Errorf("--to is required (destination notebook)")
		}

		sshT, err := getSSH()
		if err != nil {
			return err
		}
		defer sshT.Close()

		// resolve source and destination notebooks
		srcDoc, err := findDoc(sshT, args[0])
		if err != nil {
			return err
		}
		dstDoc, err := findDoc(sshT, destName)
		if err != nil {
			return err
		}

		// read source content
		srcContent, srcRawContent, err := readContent(sshT, srcDoc.ID)
		if err != nil {
			return err
		}
		srcPages := srcContent.GetCPages()
		idx := pageNum - 1
		if idx < 0 || idx >= len(srcPages) {
			return fmt.Errorf("page %d does not exist (source has %d pages)", pageNum, len(srcPages))
		}
		srcPageID := srcPages[idx].ID

		// read destination content
		_, dstRawContent, err := readContent(sshT, dstDoc.ID)
		if err != nil {
			return err
		}

		// copy .rm file to destination with new UUID
		newPageID := uuid.New().String()
		srcRmPath := filepath.Join(xochitlPath, srcDoc.ID, srcPageID+".rm")
		dstRmPath := filepath.Join(xochitlPath, dstDoc.ID, newPageID+".rm")
		if _, err := sshT.RunCommand(fmt.Sprintf("cp %s %s", srcRmPath, dstRmPath)); err != nil {
			return fmt.Errorf("failed to copy .rm file: %w", err)
		}

		// add page to destination .content
		if err := addPageToContentAtEnd(sshT, dstDoc.ID, dstRawContent, newPageID); err != nil {
			return err
		}

		// remove page from source .content
		if err := removePageFromContent(sshT, srcDoc.ID, srcRawContent, idx); err != nil {
			return err
		}

		// delete source .rm file
		sshT.RunCommand(fmt.Sprintf("rm -f %s", srcRmPath))

		sshT.RunCommand("systemctl restart xochitl")

		output(map[string]any{
			"source":      srcDoc.Name,
			"destination": dstDoc.Name,
			"sourcePage":  pageNum,
			"newPageId":   newPageID,
			"status":      "moved",
		})
		return nil
	},
}

const xochitlPath = "/home/root/.local/share/remarkable/xochitl"

func init() {
	pagesAddCmd.Flags().String("template", "Blank", "page template (Blank, P Grid medium, etc)")
	pagesAddCmd.Flags().Int("after", 0, "insert after page N (0=append)")
	pagesRmCmd.Flags().Int("page", 0, "page to remove (1-indexed)")
	pagesMoveCmd.Flags().Int("page", 0, "page to move (1-indexed)")
	pagesMoveCmd.Flags().Int("to", 0, "target position (1-indexed)")

	// copy flags
	pagesCopyCmd.Flags().Int("page", 0, "page to copy (1-indexed)")
	pagesCopyCmd.Flags().String("to", "", "destination notebook")

	// move-to flags
	pagesMoveToCmd.Flags().Int("page", 0, "page to move (1-indexed)")
	pagesMoveToCmd.Flags().String("to", "", "destination notebook")

	pagesCmd.AddCommand(pagesAddCmd)
	pagesCmd.AddCommand(pagesRmCmd)
	pagesCmd.AddCommand(pagesMoveCmd)
	pagesCmd.AddCommand(pagesCopyCmd)
	pagesCmd.AddCommand(pagesMoveToCmd)
	rootCmd.AddCommand(pagesCmd)
}
