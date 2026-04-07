package e2e

import (
	"fmt"
	"os"
	"testing"
)

// TestMain builds the binary and verifies cloud auth before any test runs.
// Tests are skipped (not failed) if cloud auth is missing — keeps go test ./...
// safe on machines without a configured account.
func TestMain(m *testing.M) {
	if _, err := Build(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if !cloudReady() {
		fmt.Fprintln(os.Stderr, "e2e: cloud not authenticated — skipping all tests. Run `remarkable auth`.")
		os.Exit(0)
	}

	os.Exit(m.Run())
}

// cloudReady runs a single ls and checks the binary doesn't error out.
// Doesn't use the test helpers (no *testing.T available in TestMain).
func cloudReady() bool {
	t := &testing.T{}
	_, _, code := Run(t, "ls")
	return code == 0
}
