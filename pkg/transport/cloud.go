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
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// global concurrency cap shared across all CloudTransport instances
var httpSem = make(chan struct{}, 4)

// schema constants for sync v3 index blobs
const (
	schemaV3 = "3"
	schemaV4 = "4"
)

// indexEntry is one parsed line from a root or doc index blob
type indexEntry struct {
	hash     string
	docID    string // for root entries: docID; for doc-index entries: filename
	subfiles int    // number of files in the doc (root entries) or 0 (file entries)
	size     int64
}

// rootState is the parsed snapshot of the cloud root for one sync session
type rootState struct {
	schema     string       // "3" or "4"
	rootHash   string       // sha256 of the root index blob (mutable, advances per write)
	generation int64        // server generation counter
	entries    []indexEntry // child entries (one per doc/folder)
}

// CloudTransport implements document access via sync v3
type CloudTransport struct {
	tokens *auth.Tokens
	store  *auth.TokenStore
	client *http.Client

	// cached root state (one parsed snapshot, refreshed on demand)
	root   *rootState
	rootMu sync.Mutex

	// cached doc list (avoids 429 rate limits on repeated ls)
	docCache   []model.Document
	docCacheMu sync.Mutex
}

func NewCloudTransport() *CloudTransport {
	return &CloudTransport{
		store:  auth.NewTokenStore(),
		client: &http.Client{Timeout: 60 * time.Second},
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

// ListDocuments fetches docs — caches metadata so repeat calls are free.
// First call fetches root + per-doc index/metadata blobs (each cached on disk
// by content hash, so subsequent runs only re-fetch the mutable root).
func (t *CloudTransport) ListDocuments() ([]model.Document, error) {
	// return in-memory cache if available
	t.docCacheMu.Lock()
	if t.docCache != nil {
		cached := t.docCache
		t.docCacheMu.Unlock()
		return cached, nil
	}
	t.docCacheMu.Unlock()

	// try disk cache (survives across CLI invocations, 5min TTL)
	if docs, ok := loadDiskCache(); ok {
		t.docCacheMu.Lock()
		t.docCache = docs
		t.docCacheMu.Unlock()
		return docs, nil
	}

	rs, err := t.loadRootState()
	if err != nil {
		return nil, err
	}
	entries := rs.entries

	// fetch metadata in parallel — global httpSem caps in-flight requests
	docs := make([]model.Document, len(entries))
	errs := make([]error, len(entries))
	var wg sync.WaitGroup

	for i, e := range entries {
		wg.Add(1)
		go func(idx int, hash, docID string) {
			defer wg.Done()
			meta, err := t.fetchDocMeta(hash, docID)
			if err != nil {
				errs[idx] = err
				return
			}
			docs[idx] = *meta
		}(i, e.hash, e.docID)
	}
	wg.Wait()

	// fail loudly on any per-doc error — partial lists mislead agents
	for i, err := range errs {
		if err != nil {
			return nil, model.NewCLIError(model.ErrTransportUnavailable, "cloud",
				fmt.Sprintf("fetch metadata for %s: %v", entries[i].docID, err))
		}
	}

	// cache in memory + disk
	t.docCacheMu.Lock()
	t.docCache = docs
	t.docCacheMu.Unlock()
	saveDiskCache(docs)

	return docs, nil
}

// ReadFile downloads a file from the cloud blob tree
func (t *CloudTransport) ReadFile(docID, path string) (io.ReadCloser, error) {
	entryHash, err := t.getDocEntryHash(docID)
	if err != nil {
		return nil, err
	}

	docIndex, err := t.fetchBlob(entryHash)
	if err != nil {
		return nil, err
	}

	targetFile := resolveCloudPath(docID, path)
	fileHash := findFileHash(docIndex, targetFile)
	if fileHash == "" {
		return nil, model.NewCLIError(model.ErrNotFound, "cloud",
			fmt.Sprintf("file %q not found in doc %s", path, docID))
	}

	data, err := t.fetchBlob(fileHash)
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

	docIndex, err := t.fetchBlob(entryHash)
	if err != nil {
		return nil, err
	}

	metaHash := findFileHash(docIndex, docID+".metadata")
	if metaHash == "" {
		return nil, model.NewCLIError(model.ErrNotFound, "cloud",
			fmt.Sprintf("metadata not found for %s", docID))
	}

	metaBody, err := t.fetchBlob(metaHash)
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

// SyncDoc builds the doc index, uploads it, and updates the root index.
// Retries once on 412 (generation race) by re-fetching root and rebuilding.
func (t *CloudTransport) SyncDoc(docID string) error {
	for attempt := 0; attempt < 2; attempt++ {
		// ensure we have current root state (force refresh on retry)
		var rs *rootState
		var err error
		if attempt == 0 {
			rs, err = t.rootSnapshot()
		} else {
			rs, err = t.loadRootState()
		}
		if err != nil {
			return err
		}

		// 1. build the doc index: start from the current on-server index
		// (if this doc already exists) so partial updates (mv, tag, re-put
		// of one file) don't clobber the other files. Then overlay the
		// blobs tracked in this session.
		var baseEntries []indexEntry
		for _, e := range rs.entries {
			if e.docID == docID {
				body, err := t.fetchBlob(e.hash)
				if err != nil {
					return err
				}
				_, existing, err := parseIndex(body)
				if err != nil {
					return err
				}
				baseEntries = existing
				break
			}
		}
		fileEntries := mergeEntries(baseEntries, t.docIndexEntries(docID))
		docIndexBody := serializeIndex(rs.schema, docID, fileEntries, false)
		docIndexHash, err := hashIndex(rs.schema, fileEntries, docIndexBody)
		if err != nil {
			return err
		}

		// 2. upload the doc index blob
		if err := t.authPut(filesURL+"/"+docIndexHash, docIndexBody, docID+".docSchema"); err != nil {
			return err
		}

		// 3. compute aggregate size for the root entry
		var totalSize int64
		for _, fe := range fileEntries {
			totalSize += fe.size
		}

		// 4. build new root entries (replace or append this doc)
		newRootEntries := replaceOrAppend(rs.entries, indexEntry{
			hash:     docIndexHash,
			docID:    docID,
			subfiles: len(fileEntries),
			size:     totalSize,
		})

		// 5. serialize + hash the new root index, upload it
		rootIndexBody := serializeIndex(rs.schema, ".", newRootEntries, true)
		rootIndexHash, err := hashIndex(rs.schema, newRootEntries, rootIndexBody)
		if err != nil {
			return err
		}
		if err := t.authPut(filesURL+"/"+rootIndexHash, rootIndexBody, "root.docSchema"); err != nil {
			return err
		}

		// 6. atomic root pointer update — send current generation; server
		// increments and returns the new one. On 412 (race), retry.
		newGen, err := t.updateRoot(rootIndexHash, rs.generation)
		if err != nil {
			if attempt == 0 && isPreconditionFailed(err) {
				fmt.Fprintln(os.Stderr, "cloud: root generation race, retrying...")
				continue
			}
			return err
		}

		// 7. update cached root state and invalidate doc list cache
		t.rootMu.Lock()
		t.root = &rootState{
			schema:     rs.schema,
			rootHash:   rootIndexHash,
			generation: newGen,
			entries:    newRootEntries,
		}
		t.rootMu.Unlock()
		t.invalidateDocCache()
		return nil
	}
	return nil
}

// docIndexEntries snapshots the tracked files for a doc as indexEntry values.
func (t *CloudTransport) docIndexEntries(docID string) []indexEntry {
	trackedFilesMu.Lock()
	files := trackedFiles[docID]
	trackedFilesMu.Unlock()

	entries := make([]indexEntry, 0, len(files))
	for _, f := range files {
		entries = append(entries, indexEntry{
			hash:  f.hash,
			docID: f.filename,
			size:  int64(f.size),
		})
	}
	return entries
}

// mergeEntries overlays updated file entries onto a base list, matching by
// filename (the docID field for file entries). Updates replace, new entries
// are appended. Used to apply partial doc-level changes (rename, tag) without
// losing the content/pdf/metadata blobs already on the server.
func mergeEntries(base, updates []indexEntry) []indexEntry {
	if len(base) == 0 {
		return updates
	}
	byName := make(map[string]int, len(base))
	out := make([]indexEntry, len(base))
	for i, e := range base {
		out[i] = e
		byName[e.docID] = i
	}
	for _, u := range updates {
		if idx, ok := byName[u.docID]; ok {
			out[idx] = u
		} else {
			out = append(out, u)
			byName[u.docID] = len(out) - 1
		}
	}
	return out
}

// replaceOrAppend returns a new slice with the entry for docID replaced (or appended).
func replaceOrAppend(entries []indexEntry, e indexEntry) []indexEntry {
	out := make([]indexEntry, 0, len(entries)+1)
	replaced := false
	for _, x := range entries {
		if x.docID == e.docID {
			out = append(out, e)
			replaced = true
		} else {
			out = append(out, x)
		}
	}
	if !replaced {
		out = append(out, e)
	}
	return out
}

// DeleteDocument removes a doc from the root index.
// Note: this is a hard delete (removes from root entirely). The CLI's `rm`
// command may want to do a soft delete by moving to trash via SetMetadata
// instead — that's a caller decision.
func (t *CloudTransport) DeleteDocument(docID string) error {
	for attempt := 0; attempt < 2; attempt++ {
		var rs *rootState
		var err error
		if attempt == 0 {
			rs, err = t.rootSnapshot()
		} else {
			rs, err = t.loadRootState()
		}
		if err != nil {
			return err
		}

		// drop the entry for this doc
		newEntries := make([]indexEntry, 0, len(rs.entries))
		found := false
		for _, e := range rs.entries {
			if e.docID == docID {
				found = true
				continue
			}
			newEntries = append(newEntries, e)
		}
		if !found {
			return model.NewCLIError(model.ErrNotFound, "cloud",
				fmt.Sprintf("document %s not found in cloud", docID))
		}

		rootIndexBody := serializeIndex(rs.schema, ".", newEntries, true)
		rootIndexHash, err := hashIndex(rs.schema, newEntries, rootIndexBody)
		if err != nil {
			return err
		}
		if err := t.authPut(filesURL+"/"+rootIndexHash, rootIndexBody, "root.docSchema"); err != nil {
			return err
		}

		newGen, err := t.updateRoot(rootIndexHash, rs.generation)
		if err != nil {
			if attempt == 0 && isPreconditionFailed(err) {
				fmt.Fprintln(os.Stderr, "cloud: root generation race, retrying...")
				continue
			}
			return err
		}

		t.rootMu.Lock()
		t.root = &rootState{
			schema:     rs.schema,
			rootHash:   rootIndexHash,
			generation: newGen,
			entries:    newEntries,
		}
		t.rootMu.Unlock()
		t.invalidateDocCache()
		return nil
	}
	return nil
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

// --- internals ---

// loadRootState fetches the root JSON pointer + the root index blob and
// parses it into a typed snapshot. Always fetches fresh — callers cache.
func (t *CloudTransport) loadRootState() (*rootState, error) {
	body, err := t.authGet(rootURL)
	if err != nil {
		return nil, err
	}

	var root struct {
		Hash       string `json:"hash"`
		Generation int64  `json:"generation"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, model.NewCLIError(model.ErrCorruptedData, "cloud",
			fmt.Sprintf("invalid root pointer: %v", err))
	}

	// root index blob is content-addressed → safe to fetch via blob cache
	indexBody, err := t.fetchBlob(root.Hash)
	if err != nil {
		return nil, err
	}

	schema, entries, err := parseIndex(indexBody)
	if err != nil {
		return nil, err
	}

	rs := &rootState{
		schema:     schema,
		rootHash:   root.Hash,
		generation: root.Generation,
		entries:    entries,
	}

	t.rootMu.Lock()
	t.root = rs
	t.rootMu.Unlock()

	return rs, nil
}

// rootSnapshot returns the cached root state, loading it on first access.
func (t *CloudTransport) rootSnapshot() (*rootState, error) {
	t.rootMu.Lock()
	rs := t.root
	t.rootMu.Unlock()
	if rs != nil {
		return rs, nil
	}
	return t.loadRootState()
}

// getDocEntryHash returns the doc-index hash for a given docID from the cached root.
func (t *CloudTransport) getDocEntryHash(docID string) (string, error) {
	rs, err := t.rootSnapshot()
	if err != nil {
		return "", err
	}
	for _, e := range rs.entries {
		if e.docID == docID {
			return e.hash, nil
		}
	}
	// retry with a fresh fetch in case our snapshot is stale
	rs, err = t.loadRootState()
	if err != nil {
		return "", err
	}
	for _, e := range rs.entries {
		if e.docID == docID {
			return e.hash, nil
		}
	}
	return "", model.NewCLIError(model.ErrNotFound, "cloud",
		fmt.Sprintf("document %s not found in cloud", docID))
}

// parseIndex parses a root or doc index blob into entries.
// Format: first line is schema version ("3" or "4"). For v4, the second line
// is a totals row ("0:.:<count>:<totalSize>") which we skip. Each entry line
// is "<hash>:<type>:<id>:<subfiles>:<size>".
func parseIndex(body []byte) (schema string, entries []indexEntry, err error) {
	lines := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return schemaV4, nil, nil // treat empty as fresh v4
	}

	schema = strings.TrimSpace(lines[0])
	if schema != schemaV3 && schema != schemaV4 {
		return "", nil, model.NewCLIError(model.ErrCorruptedData, "cloud",
			fmt.Sprintf("unknown index schema %q", schema))
	}

	rest := lines[1:]
	// v4 has a totals row right after the schema line — "0:<label>:N:T"
	// where label is "." for root or the docID for a doc index. Detected by
	// 4 colon-separated fields starting with "0".
	if schema == schemaV4 && len(rest) > 0 {
		parts := strings.Split(rest[0], ":")
		if len(parts) == 4 && parts[0] == "0" {
			rest = rest[1:]
		}
	}

	for _, line := range rest {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 5 {
			continue
		}
		size, _ := strconv.ParseInt(parts[4], 10, 64)
		subfiles, _ := strconv.Atoi(parts[3])
		entries = append(entries, indexEntry{
			hash:     parts[0],
			docID:    parts[2],
			subfiles: subfiles,
			size:     size,
		})
	}
	return schema, entries, nil
}

// serializeIndex emits an index blob in the given schema.
// label is "." for the root index, or the docID for a doc index. An empty
// label skips the totals row (used only for v3 where no totals row exists).
// isRoot controls the per-entry type field: v3 root uses "80000000",
// everything else uses "0".
func serializeIndex(schema, label string, entries []indexEntry, isRoot bool) []byte {
	// keep entries sorted by docID — required for v3 hashing and matches rmapi
	sort.Slice(entries, func(i, j int) bool { return entries[i].docID < entries[j].docID })

	var b strings.Builder
	b.WriteString(schema)
	b.WriteByte('\n')

	// v4: emit totals row ("0:<label>:<count>:<totalSize>") for both root
	// and doc indexes. v3 indexes have no totals row.
	if schema == schemaV4 && label != "" {
		var total int64
		for _, e := range entries {
			total += e.size
		}
		fmt.Fprintf(&b, "0:%s:%d:%d\n", label, len(entries), total)
	}

	// per-entry type field: v3 root uses 80000000, everything else uses 0
	typeField := "0"
	if schema == schemaV3 && isRoot {
		typeField = "80000000"
	}

	for _, e := range entries {
		fmt.Fprintf(&b, "%s:%s:%s:%d:%d\n", e.hash, typeField, e.docID, e.subfiles, e.size)
	}
	return []byte(b.String())
}

// hashIndex computes the hash that goes into the parent (root pointer or
// parent doc-index entry).
//   - v3: sha256 over the binary-decoded child hashes, sorted by docID
//   - v4: sha256 of the serialized blob bytes (the body that gets PUT)
func hashIndex(schema string, entries []indexEntry, body []byte) (string, error) {
	if schema == schemaV4 {
		h := sha256.Sum256(body)
		return hex.EncodeToString(h[:]), nil
	}
	// v3
	sorted := make([]indexEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].docID < sorted[j].docID })

	hasher := sha256.New()
	for _, e := range sorted {
		raw, err := hex.DecodeString(e.hash)
		if err != nil {
			return "", model.NewCLIError(model.ErrCorruptedData, "cloud",
				fmt.Sprintf("bad child hash %q: %v", e.hash, err))
		}
		hasher.Write(raw)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
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
	body, err := t.fetchBlob(hash)
	if err != nil {
		return nil, err
	}

	metaHash := findFileHash(body, docID+".metadata")
	if metaHash == "" {
		return &model.Document{ID: docID, Name: docID}, nil
	}

	metaBody, err := t.fetchBlob(metaHash)
	if err != nil {
		return nil, err
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

// preconditionFailedErr signals a 412 from the root update endpoint
type preconditionFailedErr struct{ status int }

func (e *preconditionFailedErr) Error() string {
	return fmt.Sprintf("cloud returned %d (precondition failed)", e.status)
}

func isPreconditionFailed(err error) bool {
	_, ok := err.(*preconditionFailedErr)
	return ok
}

// do runs an http request through the global semaphore + retry loop.
// Retries on 429 (honoring Retry-After), 502/503/504, and transient net errors.
// Returns response body, status, and error. 412 surfaces as preconditionFailedErr.
func (t *CloudTransport) do(reqFn func() (*http.Request, error)) ([]byte, int, error) {
	const maxAttempts = 5

	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := reqFn()
		if err != nil {
			return nil, 0, err
		}

		// global concurrency cap
		httpSem <- struct{}{}
		resp, err := t.client.Do(req)
		<-httpSem

		// transient network error: backoff and retry
		if err != nil {
			if attempt == maxAttempts-1 {
				return nil, 0, model.NewCLIError(model.ErrTransportUnavailable, "cloud",
					fmt.Sprintf("cannot reach cloud: %v", err))
			}
			time.Sleep(backoffDelay(attempt, ""))
			continue
		}

		// 429 / transient 5xx: honor Retry-After then retry
		if resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504 {
			ra := resp.Header.Get("Retry-After")
			resp.Body.Close()
			if attempt == maxAttempts-1 {
				return nil, resp.StatusCode, model.NewCLIError(model.ErrTransportUnavailable, "cloud",
					fmt.Sprintf("cloud rate limited (HTTP %d) after %d retries", resp.StatusCode, maxAttempts))
			}
			time.Sleep(backoffDelay(attempt, ra))
			continue
		}

		// 412: precondition failed (root generation race)
		if resp.StatusCode == http.StatusPreconditionFailed {
			resp.Body.Close()
			return nil, resp.StatusCode, &preconditionFailedErr{status: resp.StatusCode}
		}

		// success path: 200/201/202
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
			return nil, resp.StatusCode, model.NewCLIError(model.ErrTransportUnavailable, "cloud",
				fmt.Sprintf("cloud %s %s returned %d: %s", req.Method, req.URL.Path, resp.StatusCode, string(body)))
		}

		return body, resp.StatusCode, nil
	}

	return nil, 0, model.NewCLIError(model.ErrTransportUnavailable, "cloud", "cloud request failed")
}

// backoffDelay returns the delay for a given attempt, honoring Retry-After if present
func backoffDelay(attempt int, retryAfter string) time.Duration {
	// honor Retry-After: seconds-int or HTTP-date
	if retryAfter != "" {
		if secs, err := strconv.Atoi(retryAfter); err == nil {
			return time.Duration(secs) * time.Second
		}
		if when, err := http.ParseTime(retryAfter); err == nil {
			d := time.Until(when)
			if d > 0 {
				return d
			}
		}
	}
	// exponential backoff with jitter: 1s, 2s, 4s, 8s, 16s + 0-500ms
	base := time.Duration(1<<attempt) * time.Second
	jitter := time.Duration(rand.Intn(500)) * time.Millisecond
	return base + jitter
}

func (t *CloudTransport) authGet(url string) ([]byte, error) {
	body, _, err := t.do(func() (*http.Request, error) {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+t.tokens.UserToken)
		return req, nil
	})
	return body, err
}

// authPut uploads a blob with rm-filename and rm-filesize headers
func (t *CloudTransport) authPut(url string, data []byte, rmFilename string) error {
	_, _, err := t.do(func() (*http.Request, error) {
		req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+t.tokens.UserToken)
		req.ContentLength = int64(len(data))
		// bypass Header.Set canonicalization — server requires lowercase keys
		req.Header["rm-filename"] = []string{rmFilename}
		req.Header["rm-filesize"] = []string{fmt.Sprintf("%d", len(data))}
		req.Header["x-goog-hash"] = []string{"crc32c=" + crc32cBase64(data)}
		return req, nil
	})
	return err
}

// updateRoot atomically advances the root pointer. The caller passes the
// CURRENT generation (what was last read); the server compares it to the
// stored generation and, on match, increments it and returns the new value.
// A 412 precondition-failed means our view is stale and the caller must
// re-read and retry. Returns the new generation on success.
func (t *CloudTransport) updateRoot(newHash string, currentGeneration int64) (int64, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"broadcast":  true,
		"hash":       newHash,
		"generation": currentGeneration,
	})

	respBody, _, err := t.do(func() (*http.Request, error) {
		req, err := http.NewRequest("PUT", rootURL, bytes.NewReader(reqBody))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+t.tokens.UserToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header["rm-filename"] = []string{"roothash"}
		return req, nil
	})
	if err != nil {
		return 0, err
	}

	var resp struct {
		Hash       string `json:"hash"`
		Generation int64  `json:"generation"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil || resp.Generation == 0 {
		// server may return bare text or a different shape; fall back to +1
		return currentGeneration + 1, nil
	}
	return resp.Generation, nil
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

// --- persistent content-addressed blob cache ---

// blobs are immutable (filename = sha256 of content), so we can cache forever
func blobCachePath(hash string) string {
	home, _ := os.UserHomeDir()
	if len(hash) < 2 {
		return ""
	}
	return filepath.Join(home, ".config", "remarkable-cli", "blobs", hash[:2], hash)
}

// fetchBlob returns a blob by hash, hitting the disk cache first
func (t *CloudTransport) fetchBlob(hash string) ([]byte, error) {
	path := blobCachePath(hash)
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			return data, nil
		}
	}

	data, err := t.authGet(filesURL + "/" + hash)
	if err != nil {
		return nil, err
	}

	// best-effort write to disk cache
	if path != "" {
		os.MkdirAll(filepath.Dir(path), 0700)
		os.WriteFile(path, data, 0600)
	}

	return data, nil
}

// --- disk cache for cloud doc listing ---

type diskCache struct {
	Docs      []model.Document `json:"docs"`
	Timestamp int64            `json:"ts"`
}

const diskCacheTTL = 300 // 5 minutes

func diskCachePath() string {
	home, _ := os.UserHomeDir()
	return home + "/.config/remarkable-cli/cloud-cache.json"
}

func loadDiskCache() ([]model.Document, bool) {
	data, err := os.ReadFile(diskCachePath())
	if err != nil {
		return nil, false
	}
	var cache diskCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, false
	}
	// check TTL
	if time.Now().Unix()-cache.Timestamp > diskCacheTTL {
		return nil, false
	}
	return cache.Docs, true
}

func saveDiskCache(docs []model.Document) {
	cache := diskCache{Docs: docs, Timestamp: time.Now().Unix()}
	data, _ := json.Marshal(cache)
	os.MkdirAll(filepath.Dir(diskCachePath()), 0700)
	os.WriteFile(diskCachePath(), data, 0600)
}

// invalidateDocCache clears in-memory + disk doc list cache after writes
func (t *CloudTransport) invalidateDocCache() {
	t.docCacheMu.Lock()
	t.docCache = nil
	t.docCacheMu.Unlock()
	os.Remove(diskCachePath())
}
