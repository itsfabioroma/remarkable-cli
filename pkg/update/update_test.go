package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// parseChecksum should handle goreleaser's `<hash>  <file>` format, including
// the star prefix that older shasum implementations emit in binary mode.
func TestParseChecksum(t *testing.T) {
	body := "aaa  other_file.tar.gz\n" +
		"bbb  remarkable_darwin_arm64.tar.gz\n" +
		"ccc *remarkable_linux_amd64.tar.gz\n"

	if got, ok := parseChecksum(body, "remarkable_darwin_arm64.tar.gz"); !ok || got != "bbb" {
		t.Errorf("darwin_arm64: got (%q, %v), want (bbb, true)", got, ok)
	}
	if got, ok := parseChecksum(body, "remarkable_linux_amd64.tar.gz"); !ok || got != "ccc" {
		t.Errorf("linux_amd64 (starred): got (%q, %v), want (ccc, true)", got, ok)
	}
	if _, ok := parseChecksum(body, "missing.tar.gz"); ok {
		t.Error("missing entry should not be found")
	}
}

// copyFile should replace dst atomically and preserve the 0755 mode.
func TestCopyFileAtomic(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("NEW"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("OLD"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "NEW" {
		t.Errorf("dst content: got %q want NEW", got)
	}
	info, _ := os.Stat(dst)
	if info.Mode().Perm() != 0755 {
		t.Errorf("dst mode: got %v want 0755", info.Mode().Perm())
	}
}

// activeExePath should return a non-empty path — this guards the bug where an
// update strategy would pick some other location instead of the actual binary.
func TestActiveExePath(t *testing.T) {
	p, err := activeExePath()
	if err != nil {
		t.Fatal(err)
	}
	if p == "" {
		t.Error("activeExePath returned empty")
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("activeExePath %q does not exist: %v", p, err)
	}
}

// End-to-end test for the release-download path: spin up a local HTTP server
// that serves a fake tarball + checksums, point releaseBaseURL at it, and
// verify that downloadAndReplace swaps the target file contents.
//
// This is the regression test for the real bug: "update said success but the
// file on PATH didn't change". Here we assert the file DID change.
func TestDownloadAndReplace_ReplacesTargetFile(t *testing.T) {
	dir := t.TempDir()

	// 1. build a fake tarball containing a file named "remarkable"
	fakeBin := []byte("#!/bin/sh\necho v999.0.0\n")
	archive := buildTarGz(t, "remarkable", fakeBin)
	sum := sha256.Sum256(archive)
	archiveHex := hex.EncodeToString(sum[:])

	name := archiveName()
	checksums := fmt.Sprintf("%s  %s\n", archiveHex, name)

	// 2. serve them from a local httptest server under /v9.9.9/
	mux := http.NewServeMux()
	mux.HandleFunc("/v9.9.9/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(checksums))
	})
	mux.HandleFunc("/v9.9.9/"+name, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 3. point the updater at our local server
	prev := releaseBaseURL
	releaseBaseURL = srv.URL
	defer func() { releaseBaseURL = prev }()

	// 4. seed a target file that simulates the user's PATH binary
	target := filepath.Join(dir, "remarkable")
	if err := os.WriteFile(target, []byte("OLD BINARY v0.0.1"), 0755); err != nil {
		t.Fatal(err)
	}

	// 5. run the update
	ctx := context.Background()
	msg, err := downloadAndReplace(ctx, "v9.9.9", target)
	if err != nil {
		t.Fatalf("downloadAndReplace: %v", err)
	}

	// 6. assert: the file was actually replaced
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("target vanished: %v", err)
	}
	if !bytes.Equal(got, fakeBin) {
		t.Fatalf("target not replaced\n got: %q\nwant: %q", got, fakeBin)
	}

	// 7. sanity: the status message should name the target path explicitly —
	//    no more "installed to ~/go/bin/remarkable" when that's not the PATH entry.
	if !bytes.Contains([]byte(msg), []byte(target)) {
		t.Errorf("status message %q should name target %q", msg, target)
	}

	// 8. mode should stay executable
	info, _ := os.Stat(target)
	if info.Mode().Perm() != 0755 {
		t.Errorf("target mode: got %v want 0755", info.Mode().Perm())
	}
}

// A bad checksum must fail loudly and must NOT touch the target.
func TestDownloadAndReplace_RejectsBadChecksum(t *testing.T) {
	dir := t.TempDir()

	archive := buildTarGz(t, "remarkable", []byte("fake"))
	// deliberately wrong checksum
	checksums := fmt.Sprintf("%s  %s\n", "deadbeef", archiveName())

	mux := http.NewServeMux()
	mux.HandleFunc("/v9.9.9/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(checksums))
	})
	mux.HandleFunc("/v9.9.9/"+archiveName(), func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	prev := releaseBaseURL
	releaseBaseURL = srv.URL
	defer func() { releaseBaseURL = prev }()

	target := filepath.Join(dir, "remarkable")
	original := []byte("UNTOUCHED")
	if err := os.WriteFile(target, original, 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := downloadAndReplace(context.Background(), "v9.9.9", target); err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}

	got, _ := os.ReadFile(target)
	if !bytes.Equal(got, original) {
		t.Errorf("target should not have changed on checksum failure; got %q", got)
	}
}

// archiveName should match goreleaser's template.
func TestArchiveName(t *testing.T) {
	want := fmt.Sprintf("remarkable_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	if got := archiveName(); got != want {
		t.Errorf("archiveName: got %q want %q", got, want)
	}
}

// buildTarGz creates a tar.gz in memory with a single file entry.
func buildTarGz(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{
		Name: name,
		Mode: 0755,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
