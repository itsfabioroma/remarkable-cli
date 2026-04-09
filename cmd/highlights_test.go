package cmd

import (
	"bytes"
	"encoding/base64"
	"io"
	"testing"
	"time"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
)

const syntheticHighlightRMBase64 = "cmVNYXJrYWJsZSAubGluZXMgZmlsZSwgdmVyc2lvbj02ICAgICAgICAgIBkAAAAAAQEJAQwTAAAAEBEREREREREREREREREREREBAAcAAAAAAQEAHwEBIQExABkAAAAAAAEKFAEAAAAkAAAAADQPAAAARAEAAABUAAAAABAAAAAAAQEBHwALLwAAMQFMAwAAAB8AARwAAAAAAQICHwABLAoAAAAfAAAsAgAAAAABPAUAAAAfAAAhASMAAAAAAQICHwALLBEAAAAfAAwsCQAAAAcBTGF5ZXIgMTwFAAAAHwAAIQEaAAAAAAEBBB8AAS8ADT8AAE8AAFQAAAAAbAQAAAACLwALXgAAAAABAQMfAAsvAQ4/AABPAABUAAAAAGxIAAAAASQKAAAANAsAAABECQAAAFwNAAAACwFIZWxsbyB3b3JsZGwhAAAAAQAAAAAAAPA/AAAAAAAAAEAAAAAAAAAIQAAAAAAAABBA"

type fakeHighlightsTransport struct {
	docs  []model.Document
	files map[string]map[string][]byte
}

func (f *fakeHighlightsTransport) ListDocuments() ([]model.Document, error) {
	return f.docs, nil
}

func (f *fakeHighlightsTransport) ReadFile(docID, path string) (io.ReadCloser, error) {
	if byDoc, ok := f.files[docID]; ok {
		if data, ok := byDoc[path]; ok {
			return io.NopCloser(bytes.NewReader(data)), nil
		}
	}
	return nil, io.EOF
}

func decodeSyntheticRM(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(syntheticHighlightRMBase64)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return data
}

func TestReadHighlights_PrefersRMOverLegacyJSON(t *testing.T) {
	fake := &fakeHighlightsTransport{
		files: map[string]map[string][]byte{
			"doc-1": {
				"page-1.rm":                     decodeSyntheticRM(t),
				"page-1.highlights/page-1.json": []byte(`{"highlights":[[{"start":1,"length":5,"text":"legacy text","color":1}]]}`),
				"doc-1.highlights/page-1.json":  []byte(`{"highlights":[[{"start":1,"length":5,"text":"legacy text","color":1}]]}`),
				"content":                       []byte(`{"pages":["page-1"]}`),
			},
		},
	}

	got := readHighlights(fake, "doc-1", "page-1")
	if len(got) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(got))
	}
	if got[0].Source != "rm" {
		t.Fatalf("expected rm source, got %q", got[0].Source)
	}
	if got[0].Text != "Hello world" {
		t.Fatalf("expected rm text, got %q", got[0].Text)
	}
}

func TestCollectLibraryHighlights_IncludesDocumentMetadata(t *testing.T) {
	modified := time.Date(2026, 4, 9, 10, 11, 12, 0, time.UTC)
	fake := &fakeHighlightsTransport{
		docs: []model.Document{
			{ID: "folder-1", Name: "Research", Type: model.DocTypeCollection, Parent: ""},
			{ID: "doc-1", Name: "Paper", Type: model.DocTypeDocument, Parent: "folder-1", FileType: "pdf", LastModified: modified},
		},
		files: map[string]map[string][]byte{
			"doc-1": {
				"content":   []byte(`{"pages":["page-1"]}`),
				"page-1.rm": decodeSyntheticRM(t),
			},
		},
	}

	got, err := collectLibraryHighlights(fake)
	if err != nil {
		t.Fatalf("collect library highlights: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 exported highlight, got %d", len(got))
	}

	item := got[0]
	if item.DocumentID != "doc-1" {
		t.Fatalf("expected document id doc-1, got %q", item.DocumentID)
	}
	if item.DocumentName != "Paper" {
		t.Fatalf("expected document name Paper, got %q", item.DocumentName)
	}
	if item.DocumentPath != "Research/Paper" {
		t.Fatalf("expected document path Research/Paper, got %q", item.DocumentPath)
	}
	if item.PageNumber != 1 || item.HighlightIndex != 1 {
		t.Fatalf("unexpected page/index: page=%d index=%d", item.PageNumber, item.HighlightIndex)
	}
	if item.Source != "rm" {
		t.Fatalf("expected source rm, got %q", item.Source)
	}
	if item.Start == nil || *item.Start != 10 {
		t.Fatalf("expected start 10, got %#v", item.Start)
	}
	if item.DocumentLastModified != modified.Format(time.RFC3339) {
		t.Fatalf("unexpected last modified: %q", item.DocumentLastModified)
	}
}

func TestCollectLibraryHighlights_SkipsUnreadableDocuments(t *testing.T) {
	fake := &fakeHighlightsTransport{
		docs: []model.Document{
			{ID: "doc-good", Name: "Good", Type: model.DocTypeDocument, Parent: "", FileType: "pdf"},
			{ID: "doc-bad", Name: "Bad", Type: model.DocTypeDocument, Parent: "", FileType: "pdf"},
			{ID: "doc-note", Name: "Note", Type: model.DocTypeDocument, Parent: "", FileType: ""},
		},
		files: map[string]map[string][]byte{
			"doc-good": {
				"content":   []byte(`{"pages":["page-1"]}`),
				"page-1.rm": decodeSyntheticRM(t),
			},
			"doc-bad": {
				"content": []byte(`{"pages":[]}`),
			},
			"doc-note": {
				"content":   []byte(`{"pages":["page-1"]}`),
				"page-1.rm": decodeSyntheticRM(t),
			},
		},
	}

	got, err := collectLibraryHighlights(fake)
	if err != nil {
		t.Fatalf("collect library highlights: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 exported highlight after skipping bad doc, got %d", len(got))
	}
	if got[0].DocumentID != "doc-good" {
		t.Fatalf("expected surviving document to be doc-good, got %q", got[0].DocumentID)
	}
}
