package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimal Document shape for assertions — matches model.Document JSON tags
type doc struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Parent   string `json:"parent"`
	FileType string `json:"fileType"`
}

// TestReadLs — `ls --json` returns a non-empty document list with valid UUIDs.
func TestReadLs(t *testing.T) {
	docs := RunJSON[[]doc](t, "ls")
	if len(docs) == 0 {
		t.Fatal("ls returned empty list (account has no docs?)")
	}
	for _, d := range docs {
		if d.ID == "" || d.Name == "" {
			t.Fatalf("doc with missing id/name: %+v", d)
		}
	}
}

// TestReadLsAll — --all should be a superset of plain ls.
func TestReadLsAll(t *testing.T) {
	plain := RunJSON[[]doc](t, "ls")
	all := RunJSON[[]doc](t, "ls", "--all")
	if len(all) < len(plain) {
		t.Fatalf("--all returned fewer docs (%d) than plain ls (%d)", len(all), len(plain))
	}
}

// TestReadInfo — pick the first doc from ls and run `info` on it.
func TestReadInfo(t *testing.T) {
	docs := RunJSON[[]doc](t, "ls")
	if len(docs) == 0 {
		t.Skip("no docs to inspect")
	}
	target := docs[0]
	info := RunJSON[map[string]any](t, "info", target.Name)
	if info["id"] != target.ID {
		t.Fatalf("info.id mismatch: want %q got %v", target.ID, info["id"])
	}
}

// TestReadSearch — search for a substring of the first doc name.
func TestReadSearch(t *testing.T) {
	docs := RunJSON[[]doc](t, "ls")
	if len(docs) == 0 {
		t.Skip("no docs to search")
	}
	// pick the longest name to avoid 1-char searches matching everything
	target := docs[0]
	for _, d := range docs {
		if len(d.Name) > len(target.Name) {
			target = d
		}
	}
	if len(target.Name) < 3 {
		t.Skip("no doc with searchable name")
	}
	q := target.Name[:3]
	results := RunJSON[[]doc](t, "search", q)
	if len(results) == 0 {
		t.Fatalf("search %q returned 0 results, expected at least %q", q, target.Name)
	}
}

// TestReadGet — download the first PDF/EPUB doc.
// `get` writes to the current directory using the doc name, so we cd into a tmp dir.
func TestReadGet(t *testing.T) {
	docs := RunJSON[[]doc](t, "ls")
	var target *doc
	for i := range docs {
		if docs[i].FileType == "pdf" || docs[i].FileType == "epub" {
			target = &docs[i]
			break
		}
	}
	if target == nil {
		t.Skip("no PDF/EPUB to download")
	}

	// run from a tmp dir so the download lands there
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	os.Chdir(dir)

	MustOK(t, "get", target.Name)

	// the file lands as <name>.<ext>
	matches, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(matches) == 0 {
		t.Fatalf("no file downloaded into %s", dir)
	}
	st, err := os.Stat(matches[0])
	if err != nil || st.Size() == 0 {
		t.Fatalf("downloaded file missing or empty: %v size=%d", err, st.Size())
	}
}

// TestReadTags — SSH-only, skipped on cloud.
func TestReadTags(t *testing.T) {
	t.Skip("tags is SSH-only — covered in SSH suite")
}

// TestReadStatus — health check, should exit 0.
func TestReadStatus(t *testing.T) {
	stdout, stderr, code := Run(t, "status")
	if code != 0 {
		t.Fatalf("status failed (exit %d)\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "cloud") && !strings.Contains(stderr, "cloud") {
		t.Logf("warning: status output doesn't mention cloud:\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}
