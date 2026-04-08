package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/spf13/cobra"
)

var putCmd = &cobra.Command{
	Use:   "put <file> [folder]",
	Short: "Upload a PDF or EPUB to the device",
	Long: `Upload a local PDF or EPUB to your reMarkable, optionally into a specific folder.

Requires SSH for fastest path. Cloud transport works as fallback.`,
	Example: `  remarkable put report.pdf
  remarkable put report.pdf "Work"
  remarkable put book.epub "Reading"`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		ext := strings.ToLower(filepath.Ext(filePath))
		if ext != ".pdf" && ext != ".epub" {
			err := model.NewCLIError(model.ErrUnsupported, "", "only PDF and EPUB supported")
			outputError(err)
			return err
		}

		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		// resolve parent folder
		parentID := ""
		if len(args) > 1 {
			docs, _ := t.ListDocuments()
			tree := model.NewTree(docs)
			folder, err := tree.Resolve(args[1])
			if err != nil || !folder.IsFolder() {
				err := model.NewCLIError(model.ErrNotFound, t.Name(), fmt.Sprintf("%q is not a folder", args[1]))
				outputError(err)
				return err
			}
			parentID = folder.ID
		}

		// create document on device
		docID := uuid.New().String()
		visibleName := strings.TrimSuffix(filepath.Base(filePath), ext)
		fileType := strings.TrimPrefix(ext, ".")

		// write .metadata
		meta := &model.Metadata{
			VisibleName:  visibleName,
			Type:         string(model.DocTypeDocument),
			Parent:       parentID,
			LastModified: fmt.Sprintf("%d", time.Now().UnixMilli()),
		}
		if err := t.SetMetadata(docID, meta); err != nil {
			outputError(err)
			return err
		}

		// write .content
		content := model.Content{FileType: fileType}
		contentJSON, _ := json.Marshal(content)
		if err := t.WriteFile(docID, "content", strings.NewReader(string(contentJSON))); err != nil {
			outputError(err)
			return err
		}

		// write the actual file
		// open local source file → wrap with structured envelope
		f, err := os.Open(filePath)
		if err != nil {
			code := model.ErrIO
			if os.IsNotExist(err) {
				code = model.ErrNotFound
			}
			return model.NewCLIError(code, "", fmt.Sprintf("cannot read %s: %v", filePath, err))
		}
		defer f.Close()

		if err := t.WriteFile(docID, fileType, f); err != nil {
			outputError(err)
			return err
		}

		// finalize cloud upload (builds doc + root indexes)
		syncCloudDoc(t, docID)

		output(map[string]any{"id": docID, "name": visibleName, "fileType": fileType, "status": "uploaded"})
		return nil
	},
}

func init() { rootCmd.AddCommand(putCmd) }
