package extract

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// stripHTML removes HTML tags and decodes common entities
func stripHTML(s string) string {
	re := regexp.MustCompile("<[^>]*>")
	clean := re.ReplaceAllString(s, " ")

	// decode common entities
	clean = strings.ReplaceAll(clean, "&amp;", "&")
	clean = strings.ReplaceAll(clean, "&lt;", "<")
	clean = strings.ReplaceAll(clean, "&gt;", ">")
	clean = strings.ReplaceAll(clean, "&nbsp;", " ")

	// collapse whitespace
	re2 := regexp.MustCompile(`\s+`)
	clean = re2.ReplaceAllString(clean, " ")

	return strings.TrimSpace(clean)
}

// ExtractEPUBText reads an EPUB (which is a ZIP) and extracts all text content.
// Returns the full text with chapter separators.
func ExtractEPUBText(epubData []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(epubData), int64(len(epubData)))
	if err != nil {
		return "", fmt.Errorf("open epub zip: %w", err)
	}

	var chapters []string

	// extract text from all XHTML/HTML files
	for _, f := range zr.File {
		name := strings.ToLower(f.Name)
		if !strings.HasSuffix(name, ".xhtml") && !strings.HasSuffix(name, ".html") && !strings.HasSuffix(name, ".htm") {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			continue
		}

		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}

		text := stripHTML(string(raw))
		if text != "" {
			chapters = append(chapters, text)
		}
	}

	if len(chapters) == 0 {
		return "", fmt.Errorf("no text content found in epub")
	}

	return strings.Join(chapters, "\n---\n"), nil
}
