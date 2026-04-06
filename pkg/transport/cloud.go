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

// cloud API endpoints
const (
	syncHost   = "https://internal.cloud.remarkable.com"
	rootURL    = syncHost + "/sync/v3/root"
	filesURL   = syncHost + "/sync/v3/files"

	// legacy doc storage (for listing)
	docStorageHost = "https://document-storage-production-dot-remarkable-production.appspot.com"
	docsListURL    = docStorageHost + "/document-storage/json/2/docs"
)

// CloudTransport implements FullTransport via the reMarkable Cloud API
type CloudTransport struct {
	tokens *auth.Tokens
	store  *auth.TokenStore
	client *http.Client
}

// NewCloudTransport creates a cloud transport
func NewCloudTransport() *CloudTransport {
	return &CloudTransport{
		store:  auth.NewTokenStore(),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *CloudTransport) Name() string { return "cloud" }

// Connect loads tokens and verifies cloud access
func (t *CloudTransport) Connect() error {
	tokens, err := auth.EnsureAuth(t.store)
	if err != nil {
		return err
	}
	t.tokens = tokens
	return nil
}

func (t *CloudTransport) Close() error { return nil }

// cloudDoc is the raw response from the legacy document listing API
type cloudDoc struct {
	ID              string `json:"ID"`
	Version         int    `json:"Version"`
	Message         string `json:"Message"`
	Success         bool   `json:"Success"`
	BlobURLGet      string `json:"BlobURLGet"`
	BlobURLPut      string `json:"BlobURLPut"`
	ModifiedClient  string `json:"ModifiedClient"`
	Type            string `json:"Type"`
	VissibleName    string `json:"VissibleName"` // API typo
	CurrentPage     int    `json:"CurrentPage"`
	Bookmarked      bool   `json:"Bookmarked"`
	Parent          string `json:"Parent"`
}

// ListDocuments fetches the document list via sync v3 API
func (t *CloudTransport) ListDocuments() ([]model.Document, error) {
	// step 1: get root hash
	rootHash, err := t.getRootHash()
	if err != nil {
		return nil, err
	}

	// step 2: download root index
	rootIndex, err := t.getBlob(rootHash)
	if err != nil {
		return nil, err
	}

	// step 3: parse index entries
	entries := parseRootIndex(rootIndex)

	// step 4: for each entry, fetch the doc index to get metadata
	var docs []model.Document
	for _, entry := range entries {
		// fetch doc metadata blob
		meta, err := t.fetchDocMetadata(entry.hash, entry.docID)
		if err != nil {
			continue // skip docs we can't read
		}
		docs = append(docs, *meta)
	}

	return docs, nil
}

// rootEntry is a parsed line from the root index
type rootEntry struct {
	hash      string
	entryType string
	docID     string
	fileCount int
	size      int
}

// getRootHash fetches the current root hash from sync v3
func (t *CloudTransport) getRootHash() (string, error) {
	body, err := t.authGet(rootURL)
	if err != nil {
		return "", err
	}

	var result struct {
		Hash       string `json:"hash"`
		Generation int64  `json:"generation"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", model.NewCLIError(model.ErrCorruptedData, "cloud", "invalid root response")
	}

	return result.Hash, nil
}

// getBlob downloads a blob by hash
func (t *CloudTransport) getBlob(hash string) (string, error) {
	body, err := t.authGet(filesURL + "/" + hash)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// authGet performs an authenticated GET request with auto-retry on 401
func (t *CloudTransport) authGet(url string) ([]byte, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+t.tokens.UserToken)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, model.NewCLIError(model.ErrTransportUnavailable, "cloud",
			fmt.Sprintf("cannot reach cloud API: %v", err))
	}
	defer resp.Body.Close()

	// retry on 401
	if resp.StatusCode == http.StatusUnauthorized {
		if err := t.refreshAndRetry(); err != nil {
			return nil, err
		}
		req, _ = http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+t.tokens.UserToken)
		resp, err = t.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, model.NewCLIError(model.ErrTransportUnavailable, "cloud",
			fmt.Sprintf("cloud API returned %d: %s", resp.StatusCode, string(body)))
	}

	return io.ReadAll(resp.Body)
}

// parseRootIndex parses the root index text format
// format: schema_version\nhash:type:docID:fileCount:size\n...
func parseRootIndex(index string) []rootEntry {
	lines := strings.Split(strings.TrimSpace(index), "\n")
	var entries []rootEntry

	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) < 5 {
			continue // skip header line and malformed
		}

		fileCount := 0
		size := 0
		fmt.Sscanf(parts[3], "%d", &fileCount)
		fmt.Sscanf(parts[4], "%d", &size)

		entries = append(entries, rootEntry{
			hash:      parts[0],
			entryType: parts[1],
			docID:     parts[2],
			fileCount: fileCount,
			size:      size,
		})
	}

	return entries
}

// fetchDocMetadata downloads a doc's index blob and extracts metadata
func (t *CloudTransport) fetchDocMetadata(hash, docID string) (*model.Document, error) {
	// download doc index
	docIndex, err := t.getBlob(hash)
	if err != nil {
		return nil, err
	}

	// parse doc index to find .metadata file hash
	lines := strings.Split(strings.TrimSpace(docIndex), "\n")
	var metaHash string
	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) >= 3 && strings.HasSuffix(parts[2], ".metadata") {
			metaHash = parts[0]
			break
		}
	}

	if metaHash == "" {
		// no metadata found, return basic doc info
		return &model.Document{
			ID:   docID,
			Name: docID,
			Type: model.DocTypeDocument,
		}, nil
	}

	// download and parse metadata
	metaBlob, err := t.getBlob(metaHash)
	if err != nil {
		return &model.Document{ID: docID, Name: docID}, nil
	}

	var meta model.Metadata
	if err := json.Unmarshal([]byte(metaBlob), &meta); err != nil {
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

// GetMetadata fetches metadata for a document from cloud
func (t *CloudTransport) GetMetadata(docID string) (*model.Metadata, error) {
	// the listing API returns metadata inline, so we re-fetch
	url := docsListURL + "?doc=" + docID + "&withBlob=true"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+t.tokens.UserToken)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var cloudDocs []cloudDoc
	json.NewDecoder(resp.Body).Decode(&cloudDocs)

	for _, cd := range cloudDocs {
		if cd.ID == docID {
			return &model.Metadata{
				VisibleName: cd.VissibleName,
				Type:        cd.Type,
				Parent:      cd.Parent,
				Version:     cd.Version,
			}, nil
		}
	}

	return nil, model.NewCLIError(model.ErrNotFound, "cloud", "document not found in cloud")
}

// SetMetadata updates document metadata in the cloud
func (t *CloudTransport) SetMetadata(docID string, m *model.Metadata) error {
	// TODO: implement via upload/update-status endpoint
	return model.NewCLIError(model.ErrUnsupported, "cloud", "cloud metadata update not yet implemented")
}

// ReadFile downloads a file from the cloud
func (t *CloudTransport) ReadFile(docID, path string) (io.ReadCloser, error) {
	// get the blob URL
	url := docsListURL + "?doc=" + docID + "&withBlob=true"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+t.tokens.UserToken)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var cloudDocs []cloudDoc
	json.NewDecoder(resp.Body).Decode(&cloudDocs)

	for _, cd := range cloudDocs {
		if cd.ID == docID && cd.BlobURLGet != "" {
			// download the zip blob
			blobResp, err := t.client.Get(cd.BlobURLGet)
			if err != nil {
				return nil, err
			}
			if blobResp.StatusCode != http.StatusOK {
				blobResp.Body.Close()
				return nil, model.NewCLIError(model.ErrNotFound, "cloud", "blob download failed")
			}
			return blobResp.Body, nil
		}
	}

	return nil, model.NewCLIError(model.ErrNotFound, "cloud", "document blob not found")
}

// WriteFile uploads a file to the cloud
func (t *CloudTransport) WriteFile(docID, path string, r io.Reader) error {
	// TODO: implement via upload/request + upload/update-status
	return model.NewCLIError(model.ErrUnsupported, "cloud", "cloud upload not yet implemented")
}

// DeleteDocument removes a document from the cloud
func (t *CloudTransport) DeleteDocument(docID string) error {
	body := fmt.Sprintf(`[{"ID":"%s","Version":1}]`, docID)
	req, _ := http.NewRequest("PUT", docStorageHost+"/document-storage/json/2/delete",
		strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+t.tokens.UserToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (t *CloudTransport) refreshAndRetry() error {
	tokens, err := auth.EnsureAuth(t.store)
	if err != nil {
		return err
	}
	t.tokens = tokens
	return nil
}
