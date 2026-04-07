package transport

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
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
type CloudTransport struct {
	tokens *auth.Tokens
	store  *auth.TokenStore
	client *http.Client

	// cached root state
	rootIndex      map[string]string // docID -> entry hash
	rootIndexMu    sync.Mutex
	rootHash       string // current root blob hash
	rootGeneration int64  // for atomic update
	rootRawLines   []string // raw index lines for rebuilding
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

// --- read operations ---

// ListDocuments fetches all docs via sync v3 blob tree
func (t *CloudTransport) ListDocuments() ([]model.Document, error) {
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
func (t *CloudTransport) ReadFile(docID, path string) (io.ReadCloser, error) {
	entryHash, err := t.getDocEntryHash(docID)
	if err != nil {
		return nil, err
	}

	docIndex, err := t.authGet(filesURL + "/" + entryHash)
	if err != nil {
		return nil, err
	}

	targetFile := resolveCloudPath(docID, path)
	fileHash := findFileHash(docIndex, targetFile)
	if fileHash == "" {
		return nil, model.NewCLIError(model.ErrNotFound, "cloud",
			fmt.Sprintf("file %q not found in doc %s", path, docID))
	}

	data, err := t.authGet(filesURL + "/" + fileHash)
	if err != nil {
		return nil, err
	}

	return io.NopCloser(bytes.NewReader(data)), nil
}

// GetMetadata downloads and parses a doc's .metadata blob
func (t *CloudTransport) GetMetadata(docID string) (*model.Metadata, error) {
	entryHash, err := t.getDocEntryHash(docID)
	if err != nil {
		return nil, err
	}

	docIndex, err := t.authGet(filesURL + "/" + entryHash)
	if err != nil {
		return nil, err
	}

	metaHash := findFileHash(docIndex, docID+".metadata")
	if metaHash == "" {
		return nil, model.NewCLIError(model.ErrNotFound, "cloud",
			fmt.Sprintf("metadata not found for %s", docID))
	}

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

// --- write operations ---

// WriteFile uploads a blob to cloud and tracks it for the doc index
func (t *CloudTransport) WriteFile(docID, path string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	filename := resolveCloudPath(docID, path)
	hash := sha256Hex(data)

	// upload the blob
	if err := t.authPut(filesURL+"/"+hash, data, filename); err != nil {
		return err
	}

	// track this file for doc index building
	t.trackFile(docID, hash, filename, len(data))
	return nil
}

// SetMetadata uploads .metadata blob and updates the doc + root indexes
func (t *CloudTransport) SetMetadata(docID string, m *model.Metadata) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	filename := docID + ".metadata"
	hash := sha256Hex(data)

	// upload metadata blob
	if err := t.authPut(filesURL+"/"+hash, data, filename); err != nil {
		return err
	}

	t.trackFile(docID, hash, filename, len(data))
	return nil
}

// SyncDoc builds the doc index, uploads it, and updates the root index
// call after all WriteFile/SetMetadata calls for a document
func (t *CloudTransport) SyncDoc(docID string) error {
	// ensure we have the current root state
	if t.rootHash == "" {
		if _, err := t.getRootEntries(); err != nil {
			return err
		}
	}

	// build doc index from tracked files
	docIndexBody := t.buildDocIndex(docID)
	docIndexHash := sha256Hex(docIndexBody)

	// upload doc index blob
	if err := t.authPut(filesURL+"/"+docIndexHash, docIndexBody, docID); err != nil {
		return err
	}

	// build new root index: replace or add this doc's entry
	fileCount := t.trackedFileCount(docID)
	totalSize := t.trackedTotalSize(docID)
	newEntry := fmt.Sprintf("%s:80000000:%s:%d:%d", docIndexHash, docID, fileCount, totalSize)

	var newRootLines []string
	replaced := false
	for _, line := range t.rootRawLines {
		parts := strings.Split(line, ":")
		if len(parts) >= 3 && parts[2] == docID {
			newRootLines = append(newRootLines, newEntry)
			replaced = true
		} else {
			newRootLines = append(newRootLines, line)
		}
	}
	if !replaced {
		newRootLines = append(newRootLines, newEntry)
	}

	// upload new root index blob
	rootIndexBody := []byte(strings.Join(newRootLines, "\n"))
	rootIndexHash := sha256Hex(rootIndexBody)

	if err := t.authPut(filesURL+"/"+rootIndexHash, rootIndexBody, "root"); err != nil {
		return err
	}

	// atomic root update
	return t.updateRoot(rootIndexHash, t.rootGeneration+1)
}

// DeleteDocument removes a doc from the root index
func (t *CloudTransport) DeleteDocument(docID string) error {
	// ensure we have current root state
	if t.rootHash == "" {
		if _, err := t.getRootEntries(); err != nil {
			return err
		}
	}

	// rebuild root index without this doc
	var newRootLines []string
	for _, line := range t.rootRawLines {
		parts := strings.Split(line, ":")
		if len(parts) >= 3 && parts[2] == docID {
			continue // skip this doc
		}
		newRootLines = append(newRootLines, line)
	}

	rootIndexBody := []byte(strings.Join(newRootLines, "\n"))
	rootIndexHash := sha256Hex(rootIndexBody)

	// upload new root index
	if err := t.authPut(filesURL+"/"+rootIndexHash, rootIndexBody, "root"); err != nil {
		return err
	}

	return t.updateRoot(rootIndexHash, t.rootGeneration+1)
}

// --- tracked files for doc index building ---

// trackedFile holds info about an uploaded blob
type trackedFile struct {
	hash     string
	filename string
	size     int
}

var (
	trackedFiles   = make(map[string][]trackedFile) // docID -> files
	trackedFilesMu sync.Mutex
)

func (t *CloudTransport) trackFile(docID, hash, filename string, size int) {
	trackedFilesMu.Lock()
	defer trackedFilesMu.Unlock()
	trackedFiles[docID] = append(trackedFiles[docID], trackedFile{hash, filename, size})
}

func (t *CloudTransport) buildDocIndex(docID string) []byte {
	trackedFilesMu.Lock()
	files := trackedFiles[docID]
	trackedFilesMu.Unlock()

	var lines []string
	for _, f := range files {
		lines = append(lines, fmt.Sprintf("%s:0:%s:0:%d", f.hash, f.filename, f.size))
	}
	return []byte(strings.Join(lines, "\n"))
}

func (t *CloudTransport) trackedFileCount(docID string) int {
	trackedFilesMu.Lock()
	defer trackedFilesMu.Unlock()
	return len(trackedFiles[docID])
}

func (t *CloudTransport) trackedTotalSize(docID string) int {
	trackedFilesMu.Lock()
	defer trackedFilesMu.Unlock()
	total := 0
	for _, f := range trackedFiles[docID] {
		total += f.size
	}
	return total
}

// --- internals ---

type rootEntry struct {
	hash, docID string
}

// getRootEntries fetches root index and caches hash, generation, raw lines
func (t *CloudTransport) getRootEntries() ([]rootEntry, error) {
	body, err := t.authGet(rootURL)
	if err != nil {
		return nil, err
	}

	var root struct {
		Hash       string `json:"hash"`
		Generation int64  `json:"generation"`
	}
	json.Unmarshal(body, &root)

	// save root state for writes
	t.rootHash = root.Hash
	t.rootGeneration = root.Generation

	// get root index blob
	indexBody, err := t.authGet(filesURL + "/" + root.Hash)
	if err != nil {
		return nil, err
	}

	var entries []rootEntry
	index := make(map[string]string)
	var rawLines []string

	for _, line := range strings.Split(string(indexBody), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rawLines = append(rawLines, line)
		parts := strings.Split(line, ":")
		if len(parts) < 5 {
			continue
		}
		hash := parts[0]
		docID := parts[2]
		entries = append(entries, rootEntry{hash: hash, docID: docID})
		index[docID] = hash
	}

	t.rootIndexMu.Lock()
	t.rootIndex = index
	t.rootRawLines = rawLines
	t.rootIndexMu.Unlock()

	return entries, nil
}

func (t *CloudTransport) getDocEntryHash(docID string) (string, error) {
	t.rootIndexMu.Lock()
	idx := t.rootIndex
	t.rootIndexMu.Unlock()

	if idx != nil {
		if hash, ok := idx[docID]; ok {
			return hash, nil
		}
	}

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

// resolveCloudPath maps a ReadFile/WriteFile path to the cloud filename
func resolveCloudPath(docID, path string) string {
	switch path {
	case "content", "pdf", "epub", "metadata", "pagedata":
		return docID + "." + path
	}

	if strings.Contains(path, ".") && !strings.Contains(path, "/") {
		return docID + "/" + path
	}
	if strings.Contains(path, "/") {
		return docID + "/" + path
	}

	return docID + "." + path
}

// findFileHash searches a doc/root index blob for a filename
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

func (t *CloudTransport) fetchDocMeta(hash, docID string) (*model.Document, error) {
	body, err := t.authGet(filesURL + "/" + hash)
	if err != nil {
		return nil, err
	}

	metaHash := findFileHash(body, docID+".metadata")
	if metaHash == "" {
		return &model.Document{ID: docID, Name: docID}, nil
	}

	metaBody, err := t.authGet(filesURL + "/" + metaHash)
	if err != nil {
		return &model.Document{ID: docID, Name: docID}, nil
	}

	var meta model.Metadata
	if err := json.Unmarshal(metaBody, &meta); err != nil {
		return &model.Document{ID: docID, Name: docID}, nil
	}

	fileType := inferFileType(body, docID)

	return &model.Document{
		ID:       docID,
		Name:     meta.VisibleName,
		Type:     model.DocType(meta.Type),
		Parent:   meta.Parent,
		Pinned:   meta.Pinned,
		FileType: fileType,
	}, nil
}

func inferFileType(docIndex []byte, docID string) string {
	if findFileHash(docIndex, docID+".pdf") != "" {
		return "pdf"
	}
	if findFileHash(docIndex, docID+".epub") != "" {
		return "epub"
	}
	return ""
}

// --- HTTP helpers ---

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

// authPut uploads a blob with rm-filename and rm-filesize headers
func (t *CloudTransport) authPut(url string, data []byte, rmFilename string) error {
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+t.tokens.UserToken)
	req.ContentLength = int64(len(data))
	req.Header.Set("rm-filename", rmFilename)
	req.Header.Set("rm-filesize", fmt.Sprintf("%d", len(data)))
	req.Header.Set("x-goog-hash", "crc32c="+crc32cBase64(data))

	resp, err := t.client.Do(req)
	if err != nil {
		return model.NewCLIError(model.ErrTransportUnavailable, "cloud",
			fmt.Sprintf("cloud PUT failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return model.NewCLIError(model.ErrTransportUnavailable, "cloud",
			fmt.Sprintf("cloud PUT returned %d: %s", resp.StatusCode, string(body)))
	}

	return nil
}

// updateRoot atomically updates the root hash with generation check
func (t *CloudTransport) updateRoot(newHash string, generation int64) error {
	body, _ := json.Marshal(map[string]any{
		"hash":       newHash,
		"generation": generation,
	})

	req, _ := http.NewRequest("PUT", rootURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+t.tokens.UserToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return model.NewCLIError(model.ErrTransportUnavailable, "cloud",
			fmt.Sprintf("root update failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return model.NewCLIError(model.ErrTransportUnavailable, "cloud",
			fmt.Sprintf("root update returned %d: %s", resp.StatusCode, string(respBody)))
	}

	// update cached state
	t.rootHash = newHash
	t.rootGeneration = generation

	return nil
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// crc32cBase64 returns base64-encoded CRC32C (Castagnoli) checksum
func crc32cBase64(data []byte) string {
	table := crc32.MakeTable(crc32.Castagnoli)
	c := crc32.Checksum(data, table)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, c)
	return base64.StdEncoding.EncodeToString(b)
}
