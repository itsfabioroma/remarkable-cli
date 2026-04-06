package model

import (
	"testing"
	"time"
)

func TestNewTree_RootDocuments(t *testing.T) {
	docs := []Document{
		{ID: "1", Name: "Notes", Type: DocTypeCollection, Parent: ""},
		{ID: "2", Name: "Paper.pdf", Type: DocTypeDocument, Parent: "", FileType: "pdf"},
		{ID: "3", Name: "Meeting", Type: DocTypeDocument, Parent: "1"},
	}

	tree := NewTree(docs)
	root := tree.Root()

	if len(root) != 2 {
		t.Fatalf("expected 2 root docs, got %d", len(root))
	}

	// sorted alphabetically
	if root[0].Name != "Notes" {
		t.Errorf("expected first root doc to be Notes, got %s", root[0].Name)
	}
	if root[1].Name != "Paper.pdf" {
		t.Errorf("expected second root doc to be Paper.pdf, got %s", root[1].Name)
	}
}

func TestNewTree_Children(t *testing.T) {
	docs := []Document{
		{ID: "folder1", Name: "Work", Type: DocTypeCollection, Parent: ""},
		{ID: "doc1", Name: "Report", Type: DocTypeDocument, Parent: "folder1"},
		{ID: "doc2", Name: "Agenda", Type: DocTypeDocument, Parent: "folder1"},
	}

	tree := NewTree(docs)
	children := tree.Children("folder1")

	if len(children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(children))
	}

	// sorted alphabetically
	if children[0].Name != "Agenda" {
		t.Errorf("expected Agenda first, got %s", children[0].Name)
	}
}

func TestTree_Resolve(t *testing.T) {
	docs := []Document{
		{ID: "f1", Name: "Work", Type: DocTypeCollection, Parent: ""},
		{ID: "f2", Name: "Meetings", Type: DocTypeCollection, Parent: "f1"},
		{ID: "d1", Name: "Notes", Type: DocTypeDocument, Parent: "f2"},
	}

	tree := NewTree(docs)

	// resolve nested path
	doc, err := tree.Resolve("Work/Meetings/Notes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.ID != "d1" {
		t.Errorf("expected d1, got %s", doc.ID)
	}

	// resolve folder
	folder, err := tree.Resolve("Work/Meetings")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder.ID != "f2" {
		t.Errorf("expected f2, got %s", folder.ID)
	}

	// not found
	_, err = tree.Resolve("Nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestTree_Find(t *testing.T) {
	docs := []Document{
		{ID: "1", Name: "Notes", Type: DocTypeDocument, Parent: ""},
		{ID: "2", Name: "notes", Type: DocTypeDocument, Parent: ""}, // different case
		{ID: "3", Name: "Other", Type: DocTypeDocument, Parent: ""},
	}

	tree := NewTree(docs)

	// case-insensitive search
	matches := tree.Find("notes")
	if len(matches) != 2 {
		t.Errorf("expected 2 matches, got %d", len(matches))
	}

	// no matches
	matches = tree.Find("nonexistent")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestTree_Path(t *testing.T) {
	docs := []Document{
		{ID: "f1", Name: "Work", Type: DocTypeCollection, Parent: ""},
		{ID: "f2", Name: "Projects", Type: DocTypeCollection, Parent: "f1"},
		{ID: "d1", Name: "Plan", Type: DocTypeDocument, Parent: "f2"},
	}

	tree := NewTree(docs)

	path := tree.Path("d1")
	if path != "/Work/Projects/Plan" {
		t.Errorf("expected /Work/Projects/Plan, got %s", path)
	}

	// root-level doc
	path = tree.Path("f1")
	if path != "/Work" {
		t.Errorf("expected /Work, got %s", path)
	}
}

func TestDocument_IsFolder(t *testing.T) {
	folder := Document{Type: DocTypeCollection}
	doc := Document{Type: DocTypeDocument}

	if !folder.IsFolder() {
		t.Error("expected folder to be folder")
	}
	if doc.IsFolder() {
		t.Error("expected doc to not be folder")
	}
}

// suppress unused import warning
var _ = time.Now
