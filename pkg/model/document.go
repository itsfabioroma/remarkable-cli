package model

import "time"

// Document represents a file or folder on the reMarkable
type Document struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Type           DocType   `json:"type"`
	Parent         string    `json:"parent"`
	LastModified   time.Time `json:"lastModified"`
	Pinned         bool      `json:"pinned,omitempty"`
	Version        int       `json:"version"`
	CurrentPage    int       `json:"currentPage,omitempty"`
	PageCount      int       `json:"pageCount,omitempty"`
	FileType       string    `json:"fileType,omitempty"` // "pdf", "epub", ""
	Path           string    `json:"path,omitempty"`     // tree-resolved path (info only)
	Tags           []string  `json:"tags,omitempty"`     // document tags (info only)
}

// DocType distinguishes documents from folders
type DocType string

const (
	DocTypeDocument   DocType = "DocumentType"
	DocTypeCollection DocType = "CollectionType"
)

// IsFolder returns true if this is a folder
func (d *Document) IsFolder() bool {
	return d.Type == DocTypeCollection
}

// IsTrashed returns true if the document is in the trash
func (d *Document) IsTrashed() bool {
	return d.Parent == "trash"
}
