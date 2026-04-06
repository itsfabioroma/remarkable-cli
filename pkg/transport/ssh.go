package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
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
)

// xochitl data directory on reMarkable devices
const xochitlPath = "/home/root/.local/share/remarkable/xochitl"

// screen dimensions
const (
	PPWidth  = 1632 // Paper Pro
	PPHeight = 2154
)

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

	// auth methods
	var authMethods []ssh.AuthMethod

	// try SSH key first
	if t.keyPath != "" {
		if method, err := keyAuth(t.keyPath); err == nil {
			authMethods = append(authMethods, method)
		}
	}

	// try default key locations
	if t.keyPath == "" {
		home, _ := os.UserHomeDir()
		for _, name := range []string{"id_rsa", "id_ed25519"} {
			path := filepath.Join(home, ".ssh", name)
			if method, err := keyAuth(path); err == nil {
				authMethods = append(authMethods, method)
			}
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

// ListDocuments reads all .metadata files and builds document list
func (t *SSHTransport) ListDocuments() ([]model.Document, error) {
	entries, err := t.sftpClient.ReadDir(xochitlPath)
	if err != nil {
		return nil, model.NewCLIError(model.ErrTransportUnavailable, "ssh",
			fmt.Sprintf("cannot read xochitl dir: %v", err))
	}

	var docs []model.Document
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".metadata") {
			continue
		}

		uuid := strings.TrimSuffix(name, ".metadata")
		meta, err := t.GetMetadata(uuid)
		if err != nil || meta.Deleted {
			continue
		}

		// parse lastModified (epoch ms string)
		lastMod := time.Time{}
		if ms, err := strconv.ParseInt(meta.LastModified, 10, 64); err == nil {
			lastMod = time.UnixMilli(ms)
		}

		// read .content for file type + page count
		content := t.readContent(uuid)

		docs = append(docs, model.Document{
			ID:           uuid,
			Name:         meta.VisibleName,
			Type:         model.DocType(meta.Type),
			Parent:       meta.Parent,
			LastModified: lastMod,
			Pinned:       meta.Pinned,
			Version:      meta.Version,
			FileType:     content.FileType,
			PageCount:    content.PageCount,
		})
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

// Screenshot captures the device screen
// Paper Pro uses /dev/dri/card0 (no /dev/fb0). We read the framebuffer
// via /proc/[xochitl_pid]/mem at the mapped address from /proc/[pid]/maps.
func (t *SSHTransport) Screenshot() (image.Image, error) {
	// step 1: find xochitl PID
	pid, err := t.runCommand("pidof xochitl")
	if err != nil {
		return nil, model.NewCLIError(model.ErrUnsupported, "ssh",
			fmt.Sprintf("xochitl not running: %v", err))
	}
	pid = strings.TrimSpace(pid)

	// step 2: check if /dev/fb0 exists (RM2) or /dev/dri/card0 (Paper Pro)
	hasFb0, _ := t.runCommand("test -e /dev/fb0 && echo yes || echo no")
	hasFb0 = strings.TrimSpace(hasFb0)

	if hasFb0 == "yes" {
		return t.screenshotFb0()
	}

	// Paper Pro: find the framebuffer address from /proc/[pid]/maps
	return t.screenshotDRI(pid)
}

// screenshotFb0 reads the classic /dev/fb0 framebuffer (RM2)
func (t *SSHTransport) screenshotFb0() (image.Image, error) {
	session, err := t.sshClient.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	out, err := session.Output("cat /dev/fb0")
	if err != nil {
		return nil, model.NewCLIError(model.ErrUnsupported, "ssh", "cannot read /dev/fb0")
	}

	width, height := detectDimensions(len(out))
	if width == 0 {
		return nil, model.NewCLIError(model.ErrCorruptedData, "ssh",
			fmt.Sprintf("unexpected framebuffer size: %d bytes", len(out)))
	}

	return bgraToImage(out, width, height), nil
}

// screenshotDRI reads the framebuffer via /proc/pid/mem for Paper Pro
// Paper Pro uses DRI with tiled memory buffers (1632x2154 split across multiple 1.7MB tiles)
// TODO: implement proper tile reassembly (see goMarkableStream for reference)
func (t *SSHTransport) screenshotDRI(pid string) (image.Image, error) {
	return nil, model.NewCLIError(model.ErrUnsupported, "ssh",
		"Paper Pro screenshot not yet supported (DRI tiled framebuffer requires tile reassembly). "+
			"Use 'remarkable export' to render pages as SVG instead.")
}

// bgraToImage converts raw BGRA pixel data to a Go image
func bgraToImage(data []byte, width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := (y*width + x) * 4
			if offset+3 >= len(data) {
				break
			}
			img.SetRGBA(x, y, color.RGBA{
				R: data[offset+2],
				G: data[offset+1],
				B: data[offset+0],
				A: 255, // force opaque
			})
		}
	}
	return img
}

// runCommand executes a command over SSH and returns stdout
func (t *SSHTransport) runCommand(cmd string) (string, error) {
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

// SaveScreenshot writes a screenshot to a file
func SaveScreenshot(img image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
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

// detectDimensions guesses screen size from framebuffer byte count
func detectDimensions(size int) (int, int) {
	// Paper Pro: 1632 * 2154 * 4 = 14,061,696
	if size == 1632*2154*4 {
		return 1632, 2154
	}
	// RM2 (fw 3.24+): 1404 * 1872 * 4 = 10,509,696
	if size == 1404*1872*4 {
		return 1404, 1872
	}
	// RM2 legacy: 1408 * 1872 * 2 = 5,271,552
	if size == 1408*1872*2 {
		return 1408, 1872
	}
	return 0, 0
}
