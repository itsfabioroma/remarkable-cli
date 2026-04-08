package model

import (
	"fmt"
	"sort"
	"strings"
)

// Tree builds a virtual folder hierarchy from flat document list
type Tree struct {
	docs     map[string]*Document
	children map[string][]string // parentID -> childIDs
}

// NewTree constructs a tree from a flat list of documents
func NewTree(docs []Document) *Tree {
	t := &Tree{
		docs:     make(map[string]*Document),
		children: make(map[string][]string),
	}

	for i := range docs {
		d := &docs[i]
		t.docs[d.ID] = d
		t.children[d.Parent] = append(t.children[d.Parent], d.ID)
	}

	// sort children by name
	for parent := range t.children {
		sort.Slice(t.children[parent], func(i, j int) bool {
			a := t.docs[t.children[parent][i]]
			b := t.docs[t.children[parent][j]]
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		})
	}

	return t
}

// Root returns top-level documents (parent == "")
func (t *Tree) Root() []*Document {
	return t.Children("")
}

// Children returns direct children of a given parent
func (t *Tree) Children(parentID string) []*Document {
	ids := t.children[parentID]
	result := make([]*Document, 0, len(ids))
	for _, id := range ids {
		if d, ok := t.docs[id]; ok {
			result = append(result, d)
		}
	}
	return result
}

// Resolve walks a path like "Notebooks/Work/Meeting Notes" and returns the doc
func (t *Tree) Resolve(path string) (*Document, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	parentID := ""

	for i, name := range parts {
		found := false
		for _, child := range t.Children(parentID) {
			if child.Name == name {
				if i == len(parts)-1 {
					return child, nil
				}
				if !child.IsFolder() {
					return nil, fmt.Errorf("%q is not a folder", name)
				}
				parentID = child.ID
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("not found: %q", name)
		}
	}

	return nil, fmt.Errorf("empty path")
}

// Find searches for a document by name (case-insensitive, excludes trash)
func (t *Tree) Find(name string) []*Document {
	var results []*Document
	lower := strings.ToLower(name)
	for _, d := range t.docs {
		if strings.ToLower(d.Name) == lower && !d.IsTrashed() {
			results = append(results, d)
		}
	}
	return results
}

// Path returns the full path for a document
func (t *Tree) Path(docID string) string {
	var parts []string
	current := docID

	for current != "" {
		d, ok := t.docs[current]
		if !ok {
			break
		}
		parts = append([]string{d.Name}, parts...)
		current = d.Parent
	}

	return "/" + strings.Join(parts, "/")
}
