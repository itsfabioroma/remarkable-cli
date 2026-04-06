package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fabioroma/remarkable-cli/pkg/model"
	"github.com/fabioroma/remarkable-cli/pkg/transport"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var putCmd = &cobra.Command{
	Use:   "put <file> [folder]",
	Short: "Upload a PDF or EPUB to the device",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		// validate file type
		ext := strings.ToLower(filepath.Ext(filePath))
		if ext != ".pdf" && ext != ".epub" {
			err := model.NewCLIError(model.ErrUnsupported, "",
				"only PDF and EPUB files are supported")
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
			docs, err := t.ListDocuments()
			if err != nil {
				outputError(err)
				return err
			}
			tree := model.NewTree(docs)
			folder, err := tree.Resolve(args[1])
			if err != nil {
				outputError(err)
				return err
			}
			if !folder.IsFolder() {
				err := model.NewCLIError(model.ErrNotFound, t.Name(),
					fmt.Sprintf("%q is not a folder", args[1]))
				outputError(err)
				return err
			}
			parentID = folder.ID
		}

		// check if transport supports full write (SSH/Cloud)
		if fullT, ok := t.(transport.FullTransport); ok {
			return uploadViaSFTP(fullT, filePath, parentID, ext)
		}

		// USB transport: use simple upload endpoint
		if usbT, ok := t.(*transport.USBTransport); ok {
			return usbT.UploadFile(filePath)
		}

		err = model.NewCLIError(model.ErrUnsupported, t.Name(),
			"this transport does not support uploads")
		outputError(err)
		return err
	},
}

func uploadViaSFTP(t transport.FullTransport, filePath, parentID, ext string) error {
	// generate UUID for new document
	docID := uuid.New().String()
	visibleName := strings.TrimSuffix(filepath.Base(filePath), ext)
	fileType := strings.TrimPrefix(ext, ".")

	// write .metadata
	meta := &model.Metadata{
		VisibleName: visibleName,
		Type:        string(model.DocTypeDocument),
		Parent:      parentID,
		LastModified: fmt.Sprintf("%d", os.Getpagesize()), // will be set properly
		Version:     0,
	}
	if err := t.SetMetadata(docID, meta); err != nil {
		return err
	}

	// write .content
	content := model.Content{FileType: fileType}
	contentJSON, _ := json.Marshal(content)
	contentReader := strings.NewReader(string(contentJSON))
	// write as docID.content (top-level file)
	if err := writeTopLevelFile(t, docID, "content", contentReader); err != nil {
		return err
	}

	// write the actual file
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := writeTopLevelFile(t, docID, fileType, f); err != nil {
		return err
	}

	output(map[string]any{
		"id":       docID,
		"name":     visibleName,
		"fileType": fileType,
		"status":   "uploaded",
	})

	return nil
}

// writeTopLevelFile writes a file at xochitl/{docID}.{ext}
func writeTopLevelFile(t transport.FullTransport, docID, ext string, r io.Reader) error {
	return t.WriteFile(docID, ext, r)
}

func init() {
	rootCmd.AddCommand(putCmd)
}
