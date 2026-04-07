// Package e2e drives the remarkable CLI as a subprocess against a real
// reMarkable Cloud account, asserting on its JSON output. The harness is
// shared by every test file so individual tests stay tiny.
package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// path of the built binary, set by TestMain via Build
var binPath string

// Build compiles the CLI once and returns the binary path.
// Safe to call from TestMain; subsequent calls return the cached path.
var buildOnce sync.Once
var buildErr error

func Build() (string, error) {
	buildOnce.Do(func() {
		// resolve repo root from this source file (e2e/harness.go -> ../)
		_, thisFile, _, _ := runtime.Caller(0)
		repoRoot := filepath.Dir(filepath.Dir(thisFile))
		out := filepath.Join(os.TempDir(), "remarkable-e2e")

		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = repoRoot
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			buildErr = fmt.Errorf("go build failed: %v\n%s", err, stderr.String())
			return
		}
		binPath = out
	})
	return binPath, buildErr
}

// Run executes the CLI with --transport=cloud --json and returns stdout, stderr, exit code.
// Never fails the test itself — callers decide what's expected.
func Run(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if binPath == "" {
		t.Fatal("Build() not called from TestMain")
	}

	// cloud is the CLI's default transport — no need to pass it explicitly
	full := append([]string{"--json"}, args...)
	cmd := exec.Command(binPath, full...)
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se

	err := cmd.Run()
	code = 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			t.Fatalf("exec failed: %v", err)
		}
	}
	return so.String(), se.String(), code
}

// MustOK runs the command and fails the test if exit != 0.
func MustOK(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr, code := Run(t, args...)
	if code != 0 {
		t.Fatalf("`remarkable %s` failed (exit %d)\nstdout: %s\nstderr: %s",
			strings.Join(args, " "), code, stdout, stderr)
	}
	return stdout
}

// RunJSON parses MustOK output into T.
func RunJSON[T any](t *testing.T, args ...string) T {
	t.Helper()
	stdout := MustOK(t, args...)
	var v T
	if err := json.Unmarshal([]byte(stdout), &v); err != nil {
		t.Fatalf("json parse failed for `%s`: %v\nraw: %s",
			strings.Join(args, " "), err, stdout)
	}
	return v
}

// SkipUnlessWrite skips the test unless RMCLI_E2E_WRITE=1 is set.
// Write tests mutate the real cloud account so they're opt-in.
func SkipUnlessWrite(t *testing.T) {
	t.Helper()
	if os.Getenv("RMCLI_E2E_WRITE") != "1" {
		t.Skip("write test — set RMCLI_E2E_WRITE=1 to run against real cloud account")
	}
}

// UniqueName returns a stable, sortable name with the test prefix.
// Used for test artifacts so manual cleanup is easy.
func UniqueName(prefix string) string {
	return fmt.Sprintf("rmcli-e2e-%s-%d", prefix, time.Now().UnixNano())
}
