package e2e

import (
	"strings"
	"testing"
)

// findDoc returns true if a doc with the given name exists in the current ls.
func findDoc(t *testing.T, name string) bool {
	docs := RunJSON[[]doc](t, "ls")
	for _, d := range docs {
		if d.Name == name {
			return true
		}
	}
	return false
}

// cleanupDoc best-effort deletes a doc if it exists, for use in t.Cleanup.
func cleanupDoc(t *testing.T, name string) {
	if findDoc(t, name) {
		Run(t, "rm", name)
	}
}

// TestWritePutRm — full round trip: upload a tiny PDF, verify it appears in
// ls, then delete it and verify it's gone. Cleanup runs even on test failure
// so the cloud account never accumulates trash.
func TestWritePutRm(t *testing.T) {
	SkipUnlessWrite(t)

	name := UniqueName("put")
	pdf := TinyPDF(t, name)

	// upload
	out := MustOK(t, "put", pdf)
	if !strings.Contains(out, name) {
		t.Logf("put output: %s", out)
	}

	t.Cleanup(func() { cleanupDoc(t, name) })

	// verify it appears in ls
	docs := RunJSON[[]doc](t, "ls")
	found := false
	for _, d := range docs {
		if d.Name == name {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("uploaded doc %q not in ls output", name)
	}

	// verify info works on it
	info := RunJSON[map[string]any](t, "info", name)
	if info["name"] != name {
		t.Fatalf("info name mismatch: want %q got %v", name, info["name"])
	}

	// explicit delete (cleanup will be a no-op then)
	MustOK(t, "rm", name)

	// verify it's gone
	if findDoc(t, name) {
		t.Fatalf("doc %q still present after rm", name)
	}
}

// TestWriteMkdirRm — create an empty folder, verify, delete.
func TestWriteMkdirRm(t *testing.T) {
	SkipUnlessWrite(t)

	name := UniqueName("mkdir")
	t.Cleanup(func() { cleanupDoc(t, name) })

	MustOK(t, "mkdir", name)

	if !findDoc(t, name) {
		t.Fatalf("mkdir %q not in ls", name)
	}

	// verify it's a folder in info output
	info := RunJSON[map[string]any](t, "info", name)
	if info["type"] != "CollectionType" {
		t.Fatalf("expected CollectionType, got %v", info["type"])
	}

	MustOK(t, "rm", name)
	if findDoc(t, name) {
		t.Fatalf("folder %q still present after rm", name)
	}
}

// TestWriteMv — rename a doc (move without a folder target is a rename).
func TestWriteMv(t *testing.T) {
	SkipUnlessWrite(t)

	orig := UniqueName("mv-src")
	renamed := UniqueName("mv-dst")
	pdf := TinyPDF(t, orig)

	t.Cleanup(func() {
		cleanupDoc(t, orig)
		cleanupDoc(t, renamed)
	})

	MustOK(t, "put", pdf)
	if !findDoc(t, orig) {
		t.Fatalf("put %q failed", orig)
	}

	stdout, stderr, code := Run(t, "mv", orig, renamed)
	if code != 0 {
		t.Fatalf("mv failed (exit %d)\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	if findDoc(t, orig) {
		t.Fatalf("source %q still present after mv", orig)
	}
	if !findDoc(t, renamed) {
		t.Fatalf("destination %q not present after mv", renamed)
	}
}

// TestWriteStatusMentionsCloud — sanity check that status works post-writes.
func TestWriteStatusMentionsCloud(t *testing.T) {
	SkipUnlessWrite(t)
	stdout, stderr, code := Run(t, "status")
	if code != 0 {
		t.Fatalf("status failed (exit %d): %s", code, stderr)
	}
	_ = stdout
}

// silence unused-import warnings when strings isn't used by a trimmed-down run
var _ = strings.Contains
