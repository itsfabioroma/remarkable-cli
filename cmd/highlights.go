package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/itsfabioroma/remarkable-cli/pkg/encoding/rm"
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/spf13/cobra"
)

var (
	highlightsPage     int
	highlightsMarkdown bool
	highlightsAll      bool
)

// highlight JSON structures from legacy device files
type highlightFile struct {
	Highlights [][]highlightEntry `json:"highlights"`
}

type highlightEntry struct {
	Start  int    `json:"start"`
	Length int    `json:"length"`
	Text   string `json:"text"`
	Color  int    `json:"color,omitempty"`
}

type highlightRect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type rawHighlight struct {
	Start      *int
	Length     int
	Text       string
	Color      string
	Rectangles []highlightRect
	Source     string
}

type highlightResult struct {
	Page  int    `json:"page"`
	Text  string `json:"text"`
	Color string `json:"color,omitempty"`
}

type highlightExportResult struct {
	DocumentID           string          `json:"document_id"`
	DocumentName         string          `json:"document_name"`
	DocumentPath         string          `json:"document_path"`
	DocumentLastModified string          `json:"document_last_modified,omitempty"`
	PageNumber           int             `json:"page_number"`
	PageID               string          `json:"page_id,omitempty"`
	HighlightIndex       int             `json:"highlight_index"`
	Text                 string          `json:"text"`
	Color                string          `json:"color,omitempty"`
	Start                *int            `json:"start,omitempty"`
	Rectangles           []highlightRect `json:"rectangles,omitempty"`
	Source               string          `json:"source,omitempty"`
}

type highlightFileReader interface {
	ReadFile(string, string) (io.ReadCloser, error)
}

type highlightTransport interface {
	highlightFileReader
	ListDocuments() ([]model.Document, error)
}

var highlightsCmd = &cobra.Command{
	Use:   "highlights [name]",
	Short: "Extract text highlights from a document or the whole library",
	Long: `Extract highlighted text passages from reMarkable documents, as JSON or markdown.

The command reads scene-based page files (.rm) first and falls back to legacy
.highlights/*.json files when needed.`,
	Example: `  remarkable highlights "My Notes"
  remarkable highlights "Book" --page 5
  remarkable highlights "Book" --markdown
  remarkable highlights --all --json`,
	Args: func(cmd *cobra.Command, args []string) error {
		if highlightsAll {
			if len(args) != 0 {
				return fmt.Errorf("--all does not accept a document name")
			}
			if highlightsPage > 0 {
				return fmt.Errorf("--page cannot be used with --all")
			}
			return nil
		}
		if len(args) != 1 {
			return fmt.Errorf("accepts 1 arg(s), received %d", len(args))
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		if highlightsAll {
			results, err := collectLibraryHighlights(t)
			if err != nil {
				outputError(err)
				return err
			}
			if highlightsMarkdown {
				printLibraryHighlightsMarkdown(results)
				return nil
			}
			output(results)
			return nil
		}

		doc, err := findDoc(t, args[0])
		if err != nil {
			outputError(err)
			return err
		}
		results, err := collectDocumentHighlights(t, *doc, doc.Name, highlightsPage)
		if err != nil {
			outputError(err)
			return err
		}

		if highlightsMarkdown {
			for _, r := range results {
				fmt.Printf("> %s (page %d)\n\n", r.Text, r.PageNumber)
			}
			return nil
		}

		single := make([]highlightResult, 0, len(results))
		for _, r := range results {
			single = append(single, highlightResult{
				Page:  r.PageNumber,
				Text:  r.Text,
				Color: r.Color,
			})
		}
		output(single)
		return nil
	},
}

func collectLibraryHighlights(t highlightTransport) ([]highlightExportResult, error) {
	docs, err := t.ListDocuments()
	if err != nil {
		return nil, err
	}

	tree := model.NewTree(docs)
	var results []highlightExportResult
	for _, doc := range docs {
		if doc.IsFolder() || doc.IsTrashed() {
			continue
		}
		if doc.FileType != "pdf" && doc.FileType != "epub" {
			continue
		}

		path := strings.TrimPrefix(tree.Path(doc.ID), "/")
		docResults, err := collectDocumentHighlights(t, doc, path, 0)
		if err != nil {
			// surface per-doc failures so --all doesn't silently drop docs
			fmt.Fprintf(os.Stderr, "highlights: skip %q: %v\n", path, err)
			continue
		}
		results = append(results, docResults...)
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].DocumentPath != results[j].DocumentPath {
			return results[i].DocumentPath < results[j].DocumentPath
		}
		if results[i].PageNumber != results[j].PageNumber {
			return results[i].PageNumber < results[j].PageNumber
		}
		if results[i].Start != nil && results[j].Start != nil && *results[i].Start != *results[j].Start {
			return *results[i].Start < *results[j].Start
		}
		return results[i].HighlightIndex < results[j].HighlightIndex
	})
	return results, nil
}

func collectDocumentHighlights(
	t highlightFileReader,
	doc model.Document,
	documentPath string,
	pageFilter int,
) ([]highlightExportResult, error) {
	content, _, err := readContent(t, doc.ID)
	if err != nil {
		return nil, err
	}

	pageIDs := content.PageIDs()
	if len(pageIDs) == 0 {
		return nil, fmt.Errorf("no pages in %q", doc.Name)
	}

	startIdx := 0
	endIdx := len(pageIDs)
	if pageFilter > 0 {
		idx := pageFilter - 1
		if idx >= len(pageIDs) {
			return nil, fmt.Errorf("page %d does not exist (document has %d pages)", pageFilter, len(pageIDs))
		}
		startIdx = idx
		endIdx = idx + 1
	}

	lastModified := ""
	if !doc.LastModified.IsZero() {
		lastModified = doc.LastModified.UTC().Format(time.RFC3339)
	}

	var results []highlightExportResult
	for i := startIdx; i < endIdx; i++ {
		pageID := pageIDs[i]
		pageNum := i + 1

		entries := mergeAdjacent(readHighlights(t, doc.ID, pageID))
		for idx, e := range entries {
			results = append(results, highlightExportResult{
				DocumentID:           doc.ID,
				DocumentName:         doc.Name,
				DocumentPath:         documentPath,
				DocumentLastModified: lastModified,
				PageNumber:           pageNum,
				PageID:               pageID,
				HighlightIndex:       idx + 1,
				Text:                 e.Text,
				Color:                e.Color,
				Start:                e.Start,
				Rectangles:           e.Rectangles,
				Source:               e.Source,
			})
		}
	}

	return results, nil
}

func printLibraryHighlightsMarkdown(results []highlightExportResult) {
	currentDoc := ""
	currentPage := 0
	for _, r := range results {
		if r.DocumentPath != currentDoc {
			currentDoc = r.DocumentPath
			currentPage = 0
			fmt.Printf("## %s\n\n", r.DocumentPath)
		}
		if r.PageNumber != currentPage {
			currentPage = r.PageNumber
			fmt.Printf("### Page %d\n\n", r.PageNumber)
		}
		fmt.Printf("> %s\n\n", r.Text)
	}
}

func readHighlights(t highlightFileReader, docID, pageID string) []rawHighlight {
	if highlights := readRMHighlights(t, docID, pageID); len(highlights) > 0 {
		return highlights
	}
	return readLegacyHighlights(t, docID, pageID)
}

func readRMHighlights(t highlightFileReader, docID, pageID string) []rawHighlight {
	rc, err := t.ReadFile(docID, pageID+".rm")
	if err != nil {
		return nil
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil
	}

	blocks, err := rm.ParseBlocks(data)
	if err != nil {
		return nil
	}

	var highlights []rawHighlight
	for _, b := range blocks {
		if b.Type != rm.BlockSceneGlyphItem {
			continue
		}
		gd, ok := b.Data.(*rm.SceneGlyphData)
		if !ok || gd == nil || strings.TrimSpace(gd.Glyph.Text) == "" {
			continue
		}
		rects := make([]highlightRect, 0, len(gd.Glyph.Rects))
		for _, rect := range gd.Glyph.Rects {
			rects = append(rects, highlightRect{
				X: rect.X,
				Y: rect.Y,
				W: rect.W,
				H: rect.H,
			})
		}
		highlights = append(highlights, rawHighlight{
			Start:      gd.Glyph.Start,
			Length:     gd.Glyph.Length,
			Text:       gd.Glyph.Text,
			Color:      colorName(int(gd.Glyph.Color)),
			Rectangles: rects,
			Source:     "rm",
		})
	}

	sort.SliceStable(highlights, func(i, j int) bool {
		return compareStart(highlights[i].Start, highlights[j].Start)
	})

	return highlights
}

func readLegacyHighlights(t highlightFileReader, docID, pageID string) []rawHighlight {
	paths := []string{
		pageID + ".highlights/" + pageID + ".json",
		docID + ".highlights/" + pageID + ".json",
	}

	for _, p := range paths {
		rc, err := t.ReadFile(docID, p)
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(rc)
		rc.Close()

		var hf highlightFile
		if json.Unmarshal(data, &hf) != nil {
			continue
		}

		var all []rawHighlight
		for _, layer := range hf.Highlights {
			for _, entry := range layer {
				start := entry.Start
				all = append(all, rawHighlight{
					Start:  &start,
					Length: entry.Length,
					Text:   entry.Text,
					Color:  colorName(entry.Color),
					Source: "legacy_json",
				})
			}
		}

		sort.SliceStable(all, func(i, j int) bool {
			return compareStart(all[i].Start, all[j].Start)
		})
		return all
	}

	return nil
}

func mergeAdjacent(entries []rawHighlight) []rawHighlight {
	if len(entries) == 0 {
		return nil
	}

	var merged []rawHighlight
	cur := entries[0]

	for i := 1; i < len(entries); i++ {
		e := entries[i]

		if canMerge(cur, e) {
			cur.Text = strings.TrimSpace(cur.Text) + " " + strings.TrimSpace(e.Text)
			cur.Length = highlightEnd(e) - highlightStart(cur)
			cur.Rectangles = append(cur.Rectangles, e.Rectangles...)
			if cur.Color == "" {
				cur.Color = e.Color
			}
		} else {
			merged = append(merged, cur)
			cur = e
		}
	}

	merged = append(merged, cur)
	return merged
}

func compareStart(a, b *int) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return *a < *b
}

func canMerge(a, b rawHighlight) bool {
	if a.Start == nil || b.Start == nil {
		return false
	}
	if a.Color != b.Color {
		return false
	}
	gap := *b.Start - highlightEnd(a)
	return gap <= 3
}

func highlightStart(h rawHighlight) int {
	if h.Start == nil {
		return 0
	}
	return *h.Start
}

// highlightEnd assumes h.Start != nil — callers must guard (see canMerge).
func highlightEnd(h rawHighlight) int {
	return *h.Start + h.Length
}

func colorName(c int) string {
	switch c {
	case 1:
		return "yellow"
	case 2:
		return "green"
	case 3:
		return "blue"
	case 4:
		return "pink"
	case 5:
		return "purple"
	case 9:
		return "highlight"
	default:
		return "yellow"
	}
}

func init() {
	highlightsCmd.Flags().IntVar(&highlightsPage, "page", 0, "extract from single page (1-indexed)")
	highlightsCmd.Flags().BoolVar(&highlightsMarkdown, "markdown", false, "output as markdown blockquotes")
	highlightsCmd.Flags().BoolVar(&highlightsAll, "all", false, "export highlights across the whole library")
	rootCmd.AddCommand(highlightsCmd)
}
