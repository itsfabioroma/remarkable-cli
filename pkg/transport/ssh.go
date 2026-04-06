package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fabioroma/remarkable-cli/pkg/model"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// xochitl data directory on reMarkable devices
const xochitlPath = "/home/root/.local/share/remarkable/xochitl"


// SSHTransport implements DeviceTransport via SSH/SFTP
type SSHTransport struct {
	host     string
	user     string
	password string
	keyPath  string

	sshClient  *ssh.Client
	sftpClient *sftp.Client
}

// SSHOption configures the SSH transport
type SSHOption func(*SSHTransport)

// WithHost sets the SSH host (default: 10.11.99.1)
func WithHost(host string) SSHOption {
	return func(t *SSHTransport) { t.host = host }
}

// WithPassword sets password auth
func WithPassword(pw string) SSHOption {
	return func(t *SSHTransport) { t.password = pw }
}

// WithKeyPath sets SSH key path
func WithKeyPath(path string) SSHOption {
	return func(t *SSHTransport) { t.keyPath = path }
}

// NewSSHTransport creates a new SSH transport
func NewSSHTransport(opts ...SSHOption) *SSHTransport {
	t := &SSHTransport{
		host: "10.11.99.1",
		user: "root",
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func (t *SSHTransport) Name() string { return "ssh" }

// Connect establishes SSH + SFTP connections
func (t *SSHTransport) Connect() error {
	config := &ssh.ClientConfig{
		User:            t.user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	// auth methods — order: explicit key, default keys, agent, password
	var authMethods []ssh.AuthMethod

	// explicit key path
	if t.keyPath != "" {
		if method, err := keyAuth(t.keyPath); err == nil {
			authMethods = append(authMethods, method)
		}
	}

	// default key locations
	if t.keyPath == "" {
		home, _ := os.UserHomeDir()
		for _, name := range []string{"id_ed25519", "id_rsa"} {
			path := filepath.Join(home, ".ssh", name)
			if method, err := keyAuth(path); err == nil {
				authMethods = append(authMethods, method)
			}
		}
	}

	// SSH agent (macOS keychain, ssh-agent)
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			agentClient := agent.NewClient(conn)
			authMethods = append(authMethods, ssh.PublicKeysCallback(agentClient.Signers))
		}
	}

	// password auth as fallback
	if t.password != "" {
		authMethods = append(authMethods, ssh.Password(t.password))
	}

	if len(authMethods) == 0 {
		return model.NewCLIError(model.ErrAuthRequired, "ssh", "no SSH key or password configured")
	}
	config.Auth = authMethods

	// dial
	addr := net.JoinHostPort(t.host, "22")
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return model.NewCLIError(model.ErrTransportUnavailable, "ssh",
			fmt.Sprintf("cannot connect to %s: %v", addr, err))
	}
	t.sshClient = client

	// open SFTP session
	sftp, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return model.NewCLIError(model.ErrTransportUnavailable, "ssh",
			fmt.Sprintf("SFTP session failed: %v", err))
	}
	t.sftpClient = sftp

	return nil
}

// Close tears down connections
func (t *SSHTransport) Close() error {
	if t.sftpClient != nil {
		t.sftpClient.Close()
	}
	if t.sshClient != nil {
		t.sshClient.Close()
	}
	return nil
}

// ListDocuments reads all metadata in a single SSH command (~1s vs 10s+ for individual SFTP reads)
func (t *SSHTransport) ListDocuments() ([]model.Document, error) {
	// bulk read: dump all .metadata and .content files in one SSH call
	raw, err := t.RunCommand(`cd /home/root/.local/share/remarkable/xochitl && for f in *.metadata; do echo "====META $(basename $f .metadata)===="; cat "$f"; done && for f in *.content; do echo "====CONT $(basename $f .content)===="; cat "$f"; done`)
	if err != nil {
		return nil, model.NewCLIError(model.ErrTransportUnavailable, "ssh",
			fmt.Sprintf("cannot read documents: %v", err))
	}

	// parse the bulk output
	metaMap := make(map[string]*model.Metadata)
	contentMap := make(map[string]*model.Content)

	sections := strings.Split(raw, "====")
	for i := 0; i < len(sections)-1; i++ {
		header := strings.TrimSpace(sections[i])
		if header == "" {
			continue
		}

		// header is "META <uuid>" or "CONT <uuid>", body is next section
		if i+1 >= len(sections) {
			break
		}
		body := sections[i+1]

		if strings.HasPrefix(header, "META ") {
			uuid := strings.TrimPrefix(header, "META ")
			var meta model.Metadata
			if err := json.Unmarshal([]byte(body), &meta); err == nil {
				metaMap[uuid] = &meta
			}
		} else if strings.HasPrefix(header, "CONT ") {
			uuid := strings.TrimPrefix(header, "CONT ")
			var content model.Content
			if err := json.Unmarshal([]byte(body), &content); err == nil {
				contentMap[uuid] = &content
			}
		}
	}

	// build document list
	var docs []model.Document
	for uuid, meta := range metaMap {
		if meta.Deleted {
			continue
		}

		lastMod := time.Time{}
		if ms, err := strconv.ParseInt(meta.LastModified, 10, 64); err == nil {
			lastMod = time.UnixMilli(ms)
		}

		doc := model.Document{
			ID:           uuid,
			Name:         meta.VisibleName,
			Type:         model.DocType(meta.Type),
			Parent:       meta.Parent,
			LastModified: lastMod,
			Pinned:       meta.Pinned,
			Version:      meta.Version,
		}

		// merge content info
		if content, ok := contentMap[uuid]; ok {
			doc.FileType = content.FileType
			doc.PageCount = content.PageCount
		}

		docs = append(docs, doc)
	}

	return docs, nil
}

// GetMetadata reads a document's .metadata JSON
func (t *SSHTransport) GetMetadata(docID string) (*model.Metadata, error) {
	path := filepath.Join(xochitlPath, docID+".metadata")
	f, err := t.sftpClient.Open(path)
	if err != nil {
		return nil, model.NewCLIError(model.ErrNotFound, "ssh",
			fmt.Sprintf("metadata not found for %s", docID))
	}
	defer f.Close()

	var meta model.Metadata
	if err := json.NewDecoder(f).Decode(&meta); err != nil {
		return nil, model.NewCLIError(model.ErrCorruptedData, "ssh",
			fmt.Sprintf("invalid metadata for %s: %v", docID, err))
	}

	return &meta, nil
}

// SetMetadata writes a document's .metadata JSON
func (t *SSHTransport) SetMetadata(docID string, m *model.Metadata) error {
	path := filepath.Join(xochitlPath, docID+".metadata")
	f, err := t.sftpClient.Create(path)
	if err != nil {
		return model.NewCLIError(model.ErrPermissionDenied, "ssh",
			fmt.Sprintf("cannot write metadata for %s: %v", docID, err))
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(m)
}

// ReadFile reads a file relative to a document's UUID space
// relPath examples:
//   "content"     -> {xochitl}/{uuid}.content (top-level doc file)
//   "pdf"         -> {xochitl}/{uuid}.pdf
//   "abc123.rm"   -> {xochitl}/{uuid}/abc123.rm (page file inside UUID dir)
//   "abc/def.json"-> {xochitl}/{uuid}/abc/def.json
func (t *SSHTransport) ReadFile(docID, relPath string) (io.ReadCloser, error) {
	// try as top-level doc file first: {uuid}.{relPath}
	topLevel := filepath.Join(xochitlPath, docID+"."+relPath)
	if _, err := t.sftpClient.Stat(topLevel); err == nil {
		f, err := t.sftpClient.Open(topLevel)
		if err == nil {
			return f, nil
		}
	}

	// try inside the UUID dir: {uuid}/{relPath}
	insideDir := filepath.Join(xochitlPath, docID, relPath)
	f, err := t.sftpClient.Open(insideDir)
	if err != nil {
		return nil, model.NewCLIError(model.ErrNotFound, "ssh",
			fmt.Sprintf("file not found: %s/%s", docID, relPath))
	}
	return f, nil
}

// WriteRawFile writes arbitrary bytes to an absolute path on the device
func (t *SSHTransport) WriteRawFile(remotePath string, data []byte) error {
	dir := filepath.Dir(remotePath)
	t.sftpClient.MkdirAll(dir)

	f, err := t.sftpClient.Create(remotePath)
	if err != nil {
		return model.NewCLIError(model.ErrPermissionDenied, "ssh",
			fmt.Sprintf("cannot write %s: %v", remotePath, err))
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

// WriteFile writes a file into a document's space
func (t *SSHTransport) WriteFile(docID, relPath string, r io.Reader) error {
	fullPath := filepath.Join(xochitlPath, docID, relPath)

	// ensure parent dir exists
	dir := filepath.Dir(fullPath)
	if err := t.sftpClient.MkdirAll(dir); err != nil {
		return model.NewCLIError(model.ErrPermissionDenied, "ssh",
			fmt.Sprintf("cannot create dir %s: %v", dir, err))
	}

	f, err := t.sftpClient.Create(fullPath)
	if err != nil {
		return model.NewCLIError(model.ErrPermissionDenied, "ssh",
			fmt.Sprintf("cannot write %s: %v", fullPath, err))
	}
	defer f.Close()

	_, err = io.Copy(f, r)
	return err
}

// DeleteDocument removes all files for a document
func (t *SSHTransport) DeleteDocument(docID string) error {
	// remove UUID directory (pages, thumbnails)
	dirPath := filepath.Join(xochitlPath, docID)
	t.removeAll(dirPath)

	// remove dot files (.metadata, .content, .pdf, .epub, etc.)
	entries, err := t.sftpClient.ReadDir(xochitlPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), docID+".") || strings.HasPrefix(entry.Name(), docID+"/") {
			t.sftpClient.Remove(filepath.Join(xochitlPath, entry.Name()))
		}
	}

	return nil
}

// RunCommand executes a command over SSH and returns stdout
func (t *SSHTransport) RunCommand(cmd string) (string, error) {
	session, err := t.sshClient.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	out, err := session.Output(cmd)
	return string(out), err
}

// RestartUI restarts the xochitl service
func (t *SSHTransport) RestartUI() error {
	session, err := t.sshClient.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	return session.Run("systemctl restart xochitl")
}

// WatchChanges polls for file changes and emits events
func (t *SSHTransport) WatchChanges(ctx context.Context) (<-chan ChangeEvent, error) {
	ch := make(chan ChangeEvent, 16)

	// snapshot current state
	lastMtimes := make(map[string]time.Time)
	entries, err := t.sftpClient.ReadDir(xochitlPath)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".metadata") {
			lastMtimes[e.Name()] = e.ModTime()
		}
	}

	// poll loop
	go func() {
		defer close(ch)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				entries, err := t.sftpClient.ReadDir(xochitlPath)
				if err != nil {
					continue // retry on transient errors
				}

				current := make(map[string]time.Time)
				for _, e := range entries {
					if !strings.HasSuffix(e.Name(), ".metadata") {
						continue
					}
					current[e.Name()] = e.ModTime()

					// check for modifications
					if prev, ok := lastMtimes[e.Name()]; ok {
						if e.ModTime().After(prev) {
							uuid := strings.TrimSuffix(e.Name(), ".metadata")
							ch <- ChangeEvent{DocID: uuid, Type: "modified"}
						}
					} else {
						uuid := strings.TrimSuffix(e.Name(), ".metadata")
						ch <- ChangeEvent{DocID: uuid, Type: "created"}
					}
				}

				// check for deletions
				for name := range lastMtimes {
					if _, ok := current[name]; !ok {
						uuid := strings.TrimSuffix(name, ".metadata")
						ch <- ChangeEvent{DocID: uuid, Type: "deleted"}
					}
				}

				lastMtimes = current
			}
		}
	}()

	return ch, nil
}

// --- helpers ---

func keyAuth(path string) (ssh.AuthMethod, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(signer), nil
}

func (t *SSHTransport) readContent(docID string) model.Content {
	path := filepath.Join(xochitlPath, docID+".content")
	f, err := t.sftpClient.Open(path)
	if err != nil {
		return model.Content{}
	}
	defer f.Close()

	var c model.Content
	json.NewDecoder(f).Decode(&c)
	return c
}

func (t *SSHTransport) removeAll(path string) {
	entries, err := t.sftpClient.ReadDir(path)
	if err != nil {
		return
	}
	for _, e := range entries {
		child := filepath.Join(path, e.Name())
		if e.IsDir() {
			t.removeAll(child)
		}
		t.sftpClient.Remove(child)
	}
	t.sftpClient.RemoveDirectory(path)
}

