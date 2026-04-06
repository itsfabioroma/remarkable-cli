package transport

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fabioroma/remarkable-cli/pkg/auth"
	"github.com/fabioroma/remarkable-cli/pkg/model"
)

// sync v3 API endpoints
const (
	syncHost = "https://internal.cloud.remarkable.com"
	rootURL  = syncHost + "/sync/v3/root"
	filesURL = syncHost + "/sync/v3/files"
)

// CloudTransport implements read-only document listing via sync v3
// for full operations (export, put, watch), use SSH
type CloudTransport struct {
	tokens *auth.Tokens
	store  *auth.TokenStore
	client *http.Client
}

func NewCloudTransport() *CloudTransport {
	return &CloudTransport{
		store:  auth.NewTokenStore(),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *CloudTransport) Name() string  { return "cloud" }
func (t *CloudTransport) Close() error  { return nil }

func (t *CloudTransport) Connect() error {
	tokens, err := auth.EnsureAuth(t.store)
	if err != nil {
		return err
	}
	t.tokens = tokens
	return nil
}

// ListDocuments fetches all docs via sync v3 blob tree
func (t *CloudTransport) ListDocuments() ([]model.Document, error) {
	// get root hash
	body, err := t.authGet(rootURL)
	if err != nil {
		return nil, err
	}

	var root struct{ Hash string `json:"hash"` }
	json.Unmarshal(body, &root)

	// get root index
	indexBody, err := t.authGet(filesURL + "/" + root.Hash)
	if err != nil {
		return nil, err
	}

	// parse entries and fetch metadata for each doc
	var docs []model.Document
	for _, line := range strings.Split(string(indexBody), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 5 {
			continue
		}

		hash, docID := parts[0], parts[2]
		meta, err := t.fetchDocMeta(hash, docID)
		if err != nil {
			continue
		}
		docs = append(docs, *meta)
	}

	return docs, nil
}

// cloud can read blobs but not individual files within a doc
func (t *CloudTransport) ReadFile(docID, path string) (io.ReadCloser, error) {
	return nil, model.NewCLIError(model.ErrUnsupported, "cloud",
		"reading individual files requires SSH.\n  remarkable connect <host>    # add SSH for export, highlights, etc.")
}

func (t *CloudTransport) WriteFile(docID, path string, r io.Reader) error {
	return model.NewCLIError(model.ErrUnsupported, "cloud",
		"writing files requires SSH.\n  remarkable connect <host>    # add SSH for full write access")
}

func (t *CloudTransport) DeleteDocument(docID string) error {
	return model.NewCLIError(model.ErrUnsupported, "cloud",
		"deletion requires SSH.\n  remarkable connect <host>    # add SSH for delete")
}

func (t *CloudTransport) GetMetadata(docID string) (*model.Metadata, error) {
	return nil, model.NewCLIError(model.ErrUnsupported, "cloud",
		"metadata access requires SSH.\n  remarkable connect <host>    # add SSH")
}

func (t *CloudTransport) SetMetadata(docID string, m *model.Metadata) error {
	return model.NewCLIError(model.ErrUnsupported, "cloud",
		"metadata writes require SSH.\n  remarkable connect <host>    # add SSH")
}

// --- internals ---

func (t *CloudTransport) authGet(url string) ([]byte, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+t.tokens.UserToken)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, model.NewCLIError(model.ErrTransportUnavailable, "cloud",
			fmt.Sprintf("cannot reach cloud: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, model.NewCLIError(model.ErrTransportUnavailable, "cloud",
			fmt.Sprintf("cloud returned %d", resp.StatusCode))
	}

	return io.ReadAll(resp.Body)
}

func (t *CloudTransport) fetchDocMeta(hash, docID string) (*model.Document, error) {
	// download doc index blob
	body, err := t.authGet(filesURL + "/" + hash)
	if err != nil {
		return nil, err
	}

	// find .metadata hash in the doc index
	var metaHash string
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) >= 3 && strings.HasSuffix(parts[2], ".metadata") {
			metaHash = parts[0]
			break
		}
	}

	if metaHash == "" {
		return &model.Document{ID: docID, Name: docID}, nil
	}

	// download and parse metadata
	metaBody, err := t.authGet(filesURL + "/" + metaHash)
	if err != nil {
		return &model.Document{ID: docID, Name: docID}, nil
	}

	var meta model.Metadata
	if err := json.Unmarshal(metaBody, &meta); err != nil {
		return &model.Document{ID: docID, Name: docID}, nil
	}

	return &model.Document{
		ID:     docID,
		Name:   meta.VisibleName,
		Type:   model.DocType(meta.Type),
		Parent: meta.Parent,
		Pinned: meta.Pinned,
	}, nil
}
