package transport

import (
	"context"
	"io"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
)

// Transport is the single interface for device communication
type Transport interface {
	Connect() error
	Close() error
	Name() string

	// documents
	ListDocuments() ([]model.Document, error)
	ReadFile(docID, path string) (io.ReadCloser, error)
	WriteFile(docID, path string, r io.Reader) error
	DeleteDocument(docID string) error

	// metadata
	GetMetadata(docID string) (*model.Metadata, error)
	SetMetadata(docID string, m *model.Metadata) error
}

// ChangeEvent represents a document modification on the device
type ChangeEvent struct {
	DocID string
	Path  string
	Type  string // "modified", "created", "deleted"
}

// Watchable is implemented by transports that support live change detection
type Watchable interface {
	WatchChanges(ctx context.Context) (<-chan ChangeEvent, error)
}
