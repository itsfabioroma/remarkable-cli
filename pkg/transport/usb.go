package transport

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fabioroma/remarkable-cli/pkg/model"
)

// USBTransport implements Lister + Writer via the USB web interface
// Limited API: no raw .rm access, no metadata, no delete, no rename
type USBTransport struct {
	baseURL string
	client  *http.Client
}

// NewUSBTransport creates a USB web interface transport
func NewUSBTransport() *USBTransport {
	return &USBTransport{
		baseURL: "http://10.11.99.1",
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *USBTransport) Name() string { return "usb" }

// Connect verifies the USB web interface is reachable
func (t *USBTransport) Connect() error {
	resp, err := t.client.Get(t.baseURL + "/documents/")
	if err != nil {
		return model.NewCLIError(model.ErrTransportUnavailable, "usb",
			"USB web interface not reachable. Enable it in Settings > Storage > USB web interface")
	}
	resp.Body.Close()
	return nil
}

func (t *USBTransport) Close() error { return nil }

// usbDoc is the JSON response from the USB web API
type usbDoc struct {
	ID          string   `json:"ID"`
	VissibleName string  `json:"VissibleName"` // yes, the API misspells it
	Type        string   `json:"Type"`
	Parent      string   `json:"Parent"`
	ModifiedClient string `json:"ModifiedClient"`
}

// ListDocuments fetches the document list from the USB web API
func (t *USBTransport) ListDocuments() ([]model.Document, error) {
	return t.listFolder("")
}

func (t *USBTransport) listFolder(parentID string) ([]model.Document, error) {
	url := t.baseURL + "/documents/"
	if parentID != "" {
		url = t.baseURL + "/documents/" + parentID
	}

	resp, err := t.client.Get(url)
	if err != nil {
		return nil, model.NewCLIError(model.ErrTransportUnavailable, "usb",
			fmt.Sprintf("failed to list documents: %v", err))
	}
	defer resp.Body.Close()

	var usbDocs []usbDoc
	if err := json.NewDecoder(resp.Body).Decode(&usbDocs); err != nil {
		return nil, model.NewCLIError(model.ErrCorruptedData, "usb",
			fmt.Sprintf("invalid response: %v", err))
	}

	var docs []model.Document
	for _, ud := range usbDocs {
		doc := model.Document{
			ID:     ud.ID,
			Name:   ud.VissibleName,
			Type:   model.DocType(ud.Type),
			Parent: ud.Parent,
		}

		// parse ModifiedClient timestamp
		if t, err := time.Parse(time.RFC3339, ud.ModifiedClient); err == nil {
			doc.LastModified = t
		}

		docs = append(docs, doc)

		// recurse into folders
		if doc.IsFolder() {
			children, err := t.listFolder(ud.ID)
			if err == nil {
				docs = append(docs, children...)
			}
		}
	}

	return docs, nil
}

// UploadFile uploads a PDF or EPUB via the USB web interface
func (t *USBTransport) UploadFile(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	// multipart upload
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		part, _ := writer.CreateFormFile("file", filepath.Base(filePath))
		io.Copy(part, f)
		writer.Close()
		pw.Close()
	}()

	resp, err := t.client.Post(t.baseURL+"/upload", writer.FormDataContentType(), pr)
	if err != nil {
		return model.NewCLIError(model.ErrTransportUnavailable, "usb",
			fmt.Sprintf("upload failed: %v", err))
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return model.NewCLIError(model.ErrPermissionDenied, "usb",
			fmt.Sprintf("upload returned %d", resp.StatusCode))
	}

	return nil
}
