package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fabioroma/remarkable-cli/pkg/model"
	"github.com/fabioroma/remarkable-cli/pkg/transport"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var mkdirCmd = &cobra.Command{
	Use:   "mkdir <name>",
	Short: "Create a folder on the device",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		fullT, ok := t.(transport.FullTransport)
		if !ok {
			err := model.NewCLIError(model.ErrUnsupported, t.Name(),
				"this transport does not support creating folders")
			outputError(err)
			return err
		}

		// resolve parent if path contains /
		name := args[0]
		parentID := ""

		if strings.Contains(name, "/") {
			parentPath := filepath.Dir(name)
			name = filepath.Base(name)

			docs, err := t.ListDocuments()
			if err != nil {
				outputError(err)
				return err
			}
			tree := model.NewTree(docs)
			parent, err := tree.Resolve(parentPath)
			if err != nil {
				outputError(err)
				return err
			}
			parentID = parent.ID
		}

		// create folder metadata
		docID := uuid.New().String()
		meta := &model.Metadata{
			VisibleName: name,
			Type:        string(model.DocTypeCollection),
			Parent:      parentID,
			LastModified: fmt.Sprintf("%d", os.Getpagesize()),
			Version:     0,
		}

		if err := fullT.SetMetadata(docID, meta); err != nil {
			outputError(err)
			return err
		}

		output(map[string]any{
			"id":     docID,
			"name":   name,
			"parent": parentID,
			"status": "created",
		})

		return nil
	},
}

func init() {
	rootCmd.AddCommand(mkdirCmd)
}
