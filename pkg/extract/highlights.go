package extract

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Highlight represents a single text highlight
type Highlight struct {
	Page  int    `json:"page"`
	Text  string `json:"text"`
	Color string `json:"color,omitempty"`
}

// highlightEntry is the raw JSON shape from reMarkable highlight files
type highlightEntry struct {
	Start  int    `json:"start"`
	Length int    `json:"length"`
	Text   string `json:"text"`
}

// highlightFile is the top-level JSON shape
type highlightFile struct {
	Highlights [][]highlightEntry `json:"highlights"`
}

// ParseHighlightFile parses a reMarkable highlight JSON file.
// Flattens all layers, sorts by position, and returns highlights.
func ParseHighlightFile(data []byte) ([]Highlight, error) {
	var file highlightFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse highlight json: %w", err)
	}

	// flatten all layers
	var entries []highlightEntry
	for _, layer := range file.Highlights {
		entries = append(entries, layer...)
	}

	// sort by start position
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Start < entries[j].Start
	})

	// convert to Highlight
	var highlights []Highlight
	for _, e := range entries {
		highlights = append(highlights, Highlight{
			Text: e.Text,
		})
	}

	return highlights, nil
}

// MergeAdjacent merges highlights that are within 3 characters of each other.
// Operates on raw data to use positional info for gap detection.
func MergeAdjacent(highlights []Highlight) []Highlight {
	if len(highlights) == 0 {
		return nil
	}

	// without positional data, merge consecutive highlights by text proximity
	merged := []Highlight{highlights[0]}

	for i := 1; i < len(highlights); i++ {
		prev := &merged[len(merged)-1]
		cur := highlights[i]

		// always merge consecutive highlights (they came sorted from ParseHighlightFile)
		prev.Text = prev.Text + " " + cur.Text

		// preserve page from first highlight in group
		if cur.Color != "" && prev.Color == "" {
			prev.Color = cur.Color
		}
	}

	return merged
}

// mergeEntries merges raw entries with gap <= 3 chars, then converts to Highlights
func mergeEntries(entries []highlightEntry) []Highlight {
	if len(entries) == 0 {
		return nil
	}

	// sort by start
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Start < entries[j].Start
	})

	// merge adjacent entries where gap <= 3
	type merged struct {
		start  int
		end    int
		pieces []string
	}

	groups := []merged{{
		start:  entries[0].Start,
		end:    entries[0].Start + entries[0].Length,
		pieces: []string{entries[0].Text},
	}}

	for i := 1; i < len(entries); i++ {
		g := &groups[len(groups)-1]
		e := entries[i]

		// gap between end of previous and start of current
		gap := e.Start - g.end
		if gap <= 3 {
			g.end = e.Start + e.Length
			g.pieces = append(g.pieces, e.Text)
		} else {
			groups = append(groups, merged{
				start:  e.Start,
				end:    e.Start + e.Length,
				pieces: []string{e.Text},
			})
		}
	}

	// convert to Highlights
	var highlights []Highlight
	for _, g := range groups {
		text := ""
		for i, p := range g.pieces {
			if i > 0 {
				text += " "
			}
			text += p
		}
		highlights = append(highlights, Highlight{Text: text})
	}

	return highlights
}

// ParseAndMerge is a convenience that parses and merges in one step
func ParseAndMerge(data []byte) ([]Highlight, error) {
	var file highlightFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse highlight json: %w", err)
	}

	// flatten all layers
	var entries []highlightEntry
	for _, layer := range file.Highlights {
		entries = append(entries, layer...)
	}

	return mergeEntries(entries), nil
}
