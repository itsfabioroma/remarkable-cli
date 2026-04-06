package daemon

import (
	"encoding/json"
	"fmt"

	"github.com/fabioroma/remarkable-cli/pkg/model"
)

// Transport is a transport.Lister that talks to the daemon over Unix socket
// used by CLI commands when a daemon is running
type Transport struct{}

func (t *Transport) Connect() error {
	if !IsRunning() {
		return fmt.Errorf("daemon not running")
	}
	return nil
}

func (t *Transport) Close() error { return nil }
func (t *Transport) Name() string { return "daemon" }

// ListDocuments sends ls to the daemon
func (t *Transport) ListDocuments() ([]model.Document, error) {
	resp, err := SendCommand(Request{Command: "ls"})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	var docs []model.Document
	if err := json.Unmarshal(resp.Data, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}
