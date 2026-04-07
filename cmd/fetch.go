package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/spf13/cobra"
)

var fetchCmd = &cobra.Command{
	Use:   "fetch <url> [folder]",
	Short: "Download a PDF from a URL and upload to the device",
	Long: `Download a PDF from a URL and upload it directly to the reMarkable.

Validates Content-Type is application/pdf. Filename comes from Content-Disposition or URL path.`,
	Example: `  remarkable fetch https://arxiv.org/pdf/2401.12345.pdf
  remarkable fetch https://example.com/paper.pdf "Research"`,
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		url := args[0]

		// download the PDF
		client := &http.Client{Timeout: 60 * time.Second}
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			outputError(err)
			return err
		}
		req.Header.Set("User-Agent", "remarkable-cli/"+version)
		req.Header.Set("Accept", "application/pdf")

		resp, err := client.Do(req)
		if err != nil {
			outputError(err)
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			err := model.NewCLIError(model.ErrUnsupported, "", fmt.Sprintf("HTTP %d from %s", resp.StatusCode, url))
			outputError(err)
			return err
		}

		// validate content type is PDF
		ct := resp.Header.Get("Content-Type")
		mediaType, _, _ := mime.ParseMediaType(ct)
		if mediaType != "application/pdf" {
			err := model.NewCLIError(model.ErrUnsupported, "", fmt.Sprintf("only PDF URLs supported (got: %s)", ct))
			outputError(err)
			return err
		}

		// save to temp file
		tmp, err := os.CreateTemp("", "remarkable-fetch-*.pdf")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		defer tmp.Close()

		size, err := io.Copy(tmp, resp.Body)
		if err != nil {
			return err
		}
		tmp.Seek(0, 0)

		// extract filename from Content-Disposition or URL path
		visibleName := filenameFromResponse(resp, url)

		// connect to device
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
		content := model.Content{FileType: "pdf"}
		contentJSON, _ := json.Marshal(content)
		if err := t.WriteFile(docID, "content", strings.NewReader(string(contentJSON))); err != nil {
			outputError(err)
			return err
		}

		// write the PDF
		if err := t.WriteFile(docID, "pdf", tmp); err != nil {
			outputError(err)
			return err
		}

		syncCloudDoc(t, docID)

		output(map[string]any{
			"id":     docID,
			"name":   visibleName,
			"url":    url,
			"size":   size,
			"status": "fetched",
		})
		return nil
	},
}

// filenameFromResponse extracts a display name from Content-Disposition or URL path
func filenameFromResponse(resp *http.Response, url string) string {
	// try Content-Disposition header
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		_, params, err := mime.ParseMediaType(cd)
		if err == nil {
			if name, ok := params["filename"]; ok {
				return strings.TrimSuffix(name, ".pdf")
			}
		}
	}

	// fall back to URL path
	base := path.Base(url)
	base = strings.TrimSuffix(base, ".pdf")
	if base == "" || base == "." || base == "/" {
		return "fetched-document"
	}
	return base
}

func init() { rootCmd.AddCommand(fetchCmd) }
