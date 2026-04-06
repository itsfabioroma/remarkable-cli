package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/itsfabioroma/remarkable-cli/pkg/encoding/rm"
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/itsfabioroma/remarkable-cli/pkg/render"
	"github.com/itsfabioroma/remarkable-cli/pkg/transport"
	"github.com/spf13/cobra"
)

var backupRaw bool

var backupCmd = &cobra.Command{
	Use:   "backup [path]",
	Short: "Full device backup via SSH",
	Long: `Backup all documents from the reMarkable device.

  remarkable backup                    # backup to ./remarkable-backup/
  remarkable backup /path/to/dir       # custom destination
  remarkable backup --raw              # raw xochitl tar.gz (no structure)`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sshT, err := getSSH()
		if err != nil {
			outputError(err)
			return err
		}
		defer sshT.Close()

		// destination dir
		dest := "remarkable-backup"
		if len(args) > 0 {
			dest = args[0]
		}

		// raw mode: tar the xochitl dir and download
		if backupRaw {
			return backupRawMode(sshT, dest)
		}

		return backupStructured(sshT, dest)
	},
}

// backupRawMode tars the xochitl directory and downloads it
func backupRawMode(sshT *transport.SSHTransport, dest string) error {
	os.MkdirAll(dest, 0755)
	tarPath := filepath.Join(dest, "xochitl-backup.tar.gz")

	// create tar on device
	fmt.Fprintf(os.Stderr, "creating tar on device...\n")
	_, err := sshT.RunCommand("tar czf /tmp/rm-backup.tar.gz -C /home/root/.local/share/remarkable xochitl")
	if err != nil {
		return fmt.Errorf("tar failed: %w", err)
	}

	// download via RunCommand + base64 (SFTP ReadFile is doc-scoped)
	fmt.Fprintf(os.Stderr, "downloading backup...\n")
	raw, err := sshT.RunCommand("cat /tmp/rm-backup.tar.gz | base64")
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// decode and write
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("base64 decode failed: %w", err)
	}
	if err := os.WriteFile(tarPath, decoded, 0644); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	// cleanup remote
	sshT.RunCommand("rm /tmp/rm-backup.tar.gz")

	output(map[string]any{"path": tarPath, "mode": "raw"})
	return nil
}

// backupStructured downloads docs preserving folder structure
func backupStructured(sshT *transport.SSHTransport, dest string) error {
	// list all docs
	docs, err := sshT.ListDocuments()
	if err != nil {
		outputError(err)
		return err
	}

	tree := model.NewTree(docs)
	os.MkdirAll(dest, 0755)

	totalDocs := 0
	totalPages := 0

	// walk all non-trashed, non-folder docs
	for _, doc := range docs {
		if doc.IsTrashed() || doc.IsFolder() {
			continue
		}

		// build local path from tree
		treePath := tree.Path(doc.ID)
		localDir := filepath.Join(dest, filepath.FromSlash(treePath))
		os.MkdirAll(localDir, 0755)

		fmt.Fprintf(os.Stderr, "backing up: %s\n", treePath)
		totalDocs++

		// download source file (pdf/epub) if present
		if doc.FileType != "" {
			rc, err := sshT.ReadFile(doc.ID, doc.FileType)
			if err == nil {
				srcPath := filepath.Join(filepath.Dir(localDir), doc.Name+"."+doc.FileType)
				saveFile(srcPath, rc)
				rc.Close()
			}
		}

		// read .content for page UUIDs
		rc, err := sshT.ReadFile(doc.ID, "content")
		if err != nil {
			continue
		}
		var content model.Content
		json.NewDecoder(rc).Decode(&content)
		rc.Close()

		// render each page to PNG
		for i, pageID := range content.PageIDs() {
			rc, err := sshT.ReadFile(doc.ID, pageID+".rm")
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(rc)
			rc.Close()

			blocks, err := rm.ParseBlocks(data)
			if err != nil {
				continue
			}

			// render PNG
			pngPath := filepath.Join(localDir, fmt.Sprintf("page_%03d.png", i+1))
			f, err := os.Create(pngPath)
			if err != nil {
				continue
			}
			render.RenderPagePNG(f, blocks)
			f.Close()
			totalPages++
		}
	}

	output(map[string]any{
		"path":      dest,
		"documents": totalDocs,
		"pages":     totalPages,
	})
	return nil
}

// saveFile writes a ReadCloser to a local file
func saveFile(path string, rc io.ReadCloser) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, rc)
	return err
}

func init() {
	backupCmd.Flags().BoolVar(&backupRaw, "raw", false, "raw xochitl tar.gz backup (no structure)")
	rootCmd.AddCommand(backupCmd)
}
