package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

var (
	highlightsPage     int
	highlightsMarkdown bool
)

// highlight JSON structures from the device
type highlightFile struct {
	Highlights [][]highlightEntry `json:"highlights"`
}

type highlightEntry struct {
	Start  int    `json:"start"`
	Length int    `json:"length"`
	Text   string `json:"text"`
	Color  int    `json:"color,omitempty"`
}

// output entry
type highlightResult struct {
	Page  int    `json:"page"`
	Text  string `json:"text"`
	Color string `json:"color"`
}

var highlightsCmd = &cobra.Command{
	Use:   "highlights <name>",
	Short: "Extract text highlights from a document",
	Long: `Extract highlighted text passages from a document, as JSON or markdown.

Reads .highlights/*.json files written by reMarkable when you highlight in PDFs/EPUBs.`,
	Example: `  remarkable highlights "My Notes"
  remarkable highlights "Book" --page 5
  remarkable highlights "Book" --markdown`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		// find document
		doc, err := findDoc(t, args[0])
		if err != nil {
			outputError(err)
			return err
		}

		// read .content for page IDs
		content, _, err := readContent(t, doc.ID)
		if err != nil {
			outputError(err)
			return err
		}

		pageIDs := content.PageIDs()
		if len(pageIDs) == 0 {
			return fmt.Errorf("no pages in %q", doc.Name)
		}

		// filter to single page if --page set
		startIdx := 0
		endIdx := len(pageIDs)
		if highlightsPage > 0 {
			idx := highlightsPage - 1
			if idx >= len(pageIDs) {
				return fmt.Errorf("page %d does not exist (document has %d pages)", highlightsPage, len(pageIDs))
			}
			startIdx = idx
			endIdx = idx + 1
		}

		// collect highlights from each page
		var results []highlightResult

		for i := startIdx; i < endIdx; i++ {
			pageID := pageIDs[i]
			pageNum := i + 1

			entries := readHighlights(t, doc.ID, pageID)
			merged := mergeAdjacent(entries)

			for _, e := range merged {
				results = append(results, highlightResult{
					Page:  pageNum,
					Text:  e.Text,
					Color: colorName(e.Color),
				})
			}
		}

		// output
		if highlightsMarkdown {
			for _, r := range results {
				fmt.Printf("> %s (page %d)\n\n", r.Text, r.Page)
			}
			return nil
		}

		output(results)
		return nil
	},
}

// readHighlights tries known path patterns for highlight JSON
func readHighlights(t interface {
	ReadFile(string, string) (io.ReadCloser, error)
}, docID, pageID string) []highlightEntry {
	// try both known path patterns
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

		// flatten layers
		var all []highlightEntry
		for _, layer := range hf.Highlights {
			all = append(all, layer...)
		}
		return all
	}

	return nil
}

// mergeAdjacent joins highlights with gap <= 3 chars
func mergeAdjacent(entries []highlightEntry) []highlightEntry {
	if len(entries) == 0 {
		return nil
	}

	var merged []highlightEntry
	cur := entries[0]

	for i := 1; i < len(entries); i++ {
		e := entries[i]
		gap := e.Start - (cur.Start + cur.Length)

		// merge if close enough and same color
		if gap <= 3 && e.Color == cur.Color {
			cur.Length = (e.Start + e.Length) - cur.Start
			cur.Text = strings.TrimSpace(cur.Text) + " " + strings.TrimSpace(e.Text)
		} else {
			merged = append(merged, cur)
			cur = e
		}
	}
	merged = append(merged, cur)

	return merged
}

// colorName maps color index to name
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
	default:
		return "yellow"
	}
}

func init() {
	highlightsCmd.Flags().IntVar(&highlightsPage, "page", 0, "extract from single page (1-indexed)")
	highlightsCmd.Flags().BoolVar(&highlightsMarkdown, "markdown", false, "output as markdown blockquotes")
	rootCmd.AddCommand(highlightsCmd)
}
