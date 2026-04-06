package transport

import (
	"context"
	"image"
	"io"

	"github.com/fabioroma/remarkable-cli/pkg/model"
)

// Connector handles transport lifecycle
type Connector interface {
	Connect() error
	Close() error
	Name() string
}

// Lister can enumerate documents on the device
type Lister interface {
	ListDocuments() ([]model.Document, error)
}

// Reader can read files from a document's UUID directory
// path is relative, e.g. "0.rm", "0.highlights/0.json"
type Reader interface {
	ReadFile(docID, path string) (io.ReadCloser, error)
}

// Writer can write files to a document
type Writer interface {
	WriteFile(docID, path string, r io.Reader) error
}

// Deleter can remove documents
type Deleter interface {
	DeleteDocument(docID string) error
}

// MetaStore can read/write document metadata
type MetaStore interface {
	GetMetadata(docID string) (*model.Metadata, error)
	SetMetadata(docID string, m *model.Metadata) error
}

// Transport is the minimum interface all transports implement
type Transport interface {
	Connector
	Lister
}

// FullTransport has complete filesystem access (SSH, Cloud)
type FullTransport interface {
	Transport
	Reader
	Writer
	Deleter
	MetaStore
}

// ChangeEvent represents a document modification on the device
type ChangeEvent struct {
	DocID string
	Path  string
	Type  string // "modified", "created", "deleted"
}

// DeviceTransport extends FullTransport with direct device ops (SSH only)
type DeviceTransport interface {
	FullTransport
	Screenshot() (image.Image, error)
	RestartUI() error
	WatchChanges(ctx context.Context) (<-chan ChangeEvent, error)
}
