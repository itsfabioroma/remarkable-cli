package model

import (
	"encoding/json"
	"testing"
)

func TestCLIError_Error(t *testing.T) {
	// with transport
	err := NewCLIError(ErrTransportTimeout, "ssh", "connection timed out")
	expected := "[ssh] transport_timeout: connection timed out"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}

	// without transport
	err = NewCLIError(ErrNotFound, "", "document not found")
	expected = "not_found: document not found"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestCLIError_JSON(t *testing.T) {
	err := NewCLIError(ErrTransportUnavailable, "ssh", "cannot connect")

	data, jsonErr := json.Marshal(err)
	if jsonErr != nil {
		t.Fatalf("json marshal failed: %v", jsonErr)
	}

	// verify JSON structure
	var parsed map[string]string
	json.Unmarshal(data, &parsed)

	if parsed["error"] != "cannot connect" {
		t.Errorf("expected 'cannot connect', got %q", parsed["error"])
	}
	if parsed["code"] != "transport_unavailable" {
		t.Errorf("expected 'transport_unavailable', got %q", parsed["code"])
	}
	if parsed["transport"] != "ssh" {
		t.Errorf("expected 'ssh', got %q", parsed["transport"])
	}
}
