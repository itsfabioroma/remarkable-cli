package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/itsfabioroma/remarkable-cli/pkg/auth"
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
)

// sync v3 API endpoints
const (
	syncHost = "https://internal.cloud.remarkable.com"
	rootURL  = syncHost + "/sync/v3/root"
	filesURL = syncHost + "/sync/v3/files"
)

// CloudTransport implements document access via sync v3
// supports listing, reading files, and metadata
// write operations not yet implemented
type CloudTransport struct {
	tokens *auth.Tokens
	store  *auth.TokenStore
	client *http.Client


	// cached root index: docID -> entry hash
	rootIndex   map[string]string
	rootIndexMu sync.Mutex
}

func NewCloudTransport() *CloudTransport {
	return &CloudTransport{
		store:  auth.NewTokenStore(),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *CloudTransport) Name() string { return "cloud" }
func (t *CloudTransport) Close() error { return nil }

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
	// load root index (also populates cache)
	entries, err := t.getRootEntries()
	if err != nil {
		return nil, err
	}

	// fetch metadata in parallel (max 10 concurrent)
	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		sem  = make(chan struct{}, 10)
		docs []model.Document
	)

	for _, e := range entries {
		wg.Add(1)
		sem <- struct{}{}
		go func(hash, docID string) {
			defer wg.Done()
			defer func() { <-sem }()

			meta, err := t.fetchDocMeta(hash, docID)
			if err != nil {
				return
			}

			mu.Lock()
			docs = append(docs, *meta)
			mu.Unlock()
		}(e.hash, e.docID)
	}
	wg.Wait()

	return docs, nil
}

// ReadFile downloads a file from the cloud blob tree
// path examples: "pdf", "epub", "content", "pageID.rm"
func (t *CloudTransport) ReadFile(docID, path string) (io.ReadCloser, error) {
	// get the doc's entry hash from root index
	entryHash, err := t.getDocEntryHash(docID)
	if err != nil {
		return nil, err
	}

	// download doc index blob
	docIndex, err := t.authGet(filesURL + "/" + entryHash)
	if err != nil {
		return nil, err
	}

	// determine the filename to look for in the doc index
	// SSH maps: "pdf" -> {uuid}.pdf, "content" -> {uuid}.content, "abc.rm" -> {uuid}/abc.rm
	targetFile := t.resolveTargetFile(docID, path)

	// find matching hash in doc index
	fileHash := findFileHash(docIndex, targetFile)
	if fileHash == "" {
		return nil, model.NewCLIError(model.ErrNotFound, "cloud",
			fmt.Sprintf("file %q not found in doc %s", path, docID))
	}

	// download the file blob
	data, err := t.authGet(filesURL + "/" + fileHash)
	if err != nil {
		return nil, err
	}

	return io.NopCloser(bytes.NewReader(data)), nil
}

func (t *CloudTransport) WriteFile(docID, path string, r io.Reader) error {
	return model.NewCLIError(model.ErrUnsupported, "cloud",
		"cloud upload requires SSH.\n  remarkable connect <host>    # add SSH for write operations\n  cloud reads (ls, get, export, read, search) work without SSH")
}


func (t *CloudTransport) DeleteDocument(docID string) error {
	return model.NewCLIError(model.ErrUnsupported, "cloud",
		"cloud deletion requires SSH.\n  remarkable connect <host>")
}

// GetMetadata downloads and parses a doc's .metadata blob
func (t *CloudTransport) GetMetadata(docID string) (*model.Metadata, error) {
	// get entry hash
	entryHash, err := t.getDocEntryHash(docID)
	if err != nil {
		return nil, err
	}

	// download doc index blob
	docIndex, err := t.authGet(filesURL + "/" + entryHash)
	if err != nil {
		return nil, err
	}

	// find .metadata hash
	metaHash := findFileHash(docIndex, docID+".metadata")
	if metaHash == "" {
		return nil, model.NewCLIError(model.ErrNotFound, "cloud",
			fmt.Sprintf("metadata not found for %s", docID))
	}

	// download and parse
	metaBody, err := t.authGet(filesURL + "/" + metaHash)
	if err != nil {
		return nil, err
	}

	var meta model.Metadata
	if err := json.Unmarshal(metaBody, &meta); err != nil {
		return nil, model.NewCLIError(model.ErrCorruptedData, "cloud",
			fmt.Sprintf("invalid metadata for %s: %v", docID, err))
	}

	return &meta, nil
}

func (t *CloudTransport) SetMetadata(docID string, m *model.Metadata) error {
	return model.NewCLIError(model.ErrUnsupported, "cloud",
		"cloud metadata writes require SSH.\n  remarkable connect <host>")
}

// --- internals ---

// rootEntry is a parsed line from the root index
type rootEntry struct {
	hash, docID string
}

// getRootEntries fetches and caches the root index
func (t *CloudTransport) getRootEntries() ([]rootEntry, error) {
	// get root hash
	body, err := t.authGet(rootURL)
	if err != nil {
		return nil, err
	}

	var root struct {
		Hash string `json:"hash"`
	}
	json.Unmarshal(body, &root)

	// get root index blob
	indexBody, err := t.authGet(filesURL + "/" + root.Hash)
	if err != nil {
		return nil, err
	}

	// parse entries and populate cache
	var entries []rootEntry
	index := make(map[string]string)

	for _, line := range strings.Split(string(indexBody), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 5 {
			continue
		}
		hash := parts[0]
		docID := parts[2]
		entries = append(entries, rootEntry{hash: hash, docID: docID})
		index[docID] = hash
	}

	// cache the index
	t.rootIndexMu.Lock()
	t.rootIndex = index
	t.rootIndexMu.Unlock()

	return entries, nil
}

// getDocEntryHash returns the entry hash for a docID, using cache or fetching
func (t *CloudTransport) getDocEntryHash(docID string) (string, error) {
	t.rootIndexMu.Lock()
	idx := t.rootIndex
	t.rootIndexMu.Unlock()

	// use cache if available
	if idx != nil {
		if hash, ok := idx[docID]; ok {
			return hash, nil
		}
	}

	// fetch fresh root index
	_, err := t.getRootEntries()
	if err != nil {
		return "", err
	}

	t.rootIndexMu.Lock()
	hash, ok := t.rootIndex[docID]
	t.rootIndexMu.Unlock()

	if !ok {
		return "", model.NewCLIError(model.ErrNotFound, "cloud",
			fmt.Sprintf("document %s not found in cloud", docID))
	}

	return hash, nil
}

// resolveTargetFile maps a ReadFile path to the filename in the doc index
// mirrors SSH's path resolution: "pdf" -> "{uuid}.pdf", "abc.rm" -> "{uuid}/abc.rm"
func (t *CloudTransport) resolveTargetFile(docID, path string) string {
	// top-level extensions: content, pdf, epub, metadata, pagedata, etc.
	switch path {
	case "content", "pdf", "epub", "metadata", "pagedata":
		return docID + "." + path
	}

	// if it contains a dot and no slash, it's a file inside the doc dir (e.g. "abc123.rm")
	if strings.Contains(path, ".") && !strings.Contains(path, "/") {
		return docID + "/" + path
	}

	// if it contains a slash, it's already a relative path inside the doc dir
	if strings.Contains(path, "/") {
		return docID + "/" + path
	}

	// fallback: treat as extension
	return docID + "." + path
}

// findFileHash searches a doc index blob for a filename and returns its hash
// doc index format: hash:0:filename:count:size per line
func findFileHash(indexBody []byte, targetFile string) string {
	for _, line := range strings.Split(string(indexBody), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 3 {
			continue
		}
		if parts[2] == targetFile {
			return parts[0]
		}
	}
	return ""
}

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

// authPut uploads data to the cloud with authentication
func (t *CloudTransport) fetchDocMeta(hash, docID string) (*model.Document, error) {
	// download doc index blob
	body, err := t.authGet(filesURL + "/" + hash)
	if err != nil {
		return nil, err
	}

	// find .metadata hash in the doc index
	metaHash := findFileHash(body, docID+".metadata")
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
