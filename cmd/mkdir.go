package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
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

		// resolve parent if path has /
		name := args[0]
		parentID := ""

		if strings.Contains(name, "/") {
			docs, _ := t.ListDocuments()
			tree := model.NewTree(docs)
			parent, err := tree.Resolve(filepath.Dir(name))
			if err != nil {
				outputError(err)
				return err
			}
			parentID = parent.ID
			name = filepath.Base(name)
		}

		docID := uuid.New().String()
		meta := &model.Metadata{
			VisibleName:  name,
			Type:         string(model.DocTypeCollection),
			Parent:       parentID,
			LastModified: fmt.Sprintf("%d", time.Now().UnixMilli()),
		}

		if err := t.SetMetadata(docID, meta); err != nil {
			outputError(err)
			return err
		}

		syncCloudDoc(t, docID)

		output(map[string]any{"id": docID, "name": name, "parent": parentID, "status": "created"})
		return nil
	},
}

func init() { rootCmd.AddCommand(mkdirCmd) }
