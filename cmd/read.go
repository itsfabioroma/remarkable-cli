package cmd

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var readPage int

var readCmd = &cobra.Command{
	Use:   "read <name>",
	Short: "Extract text from PDFs and EPUBs on the device",
	Long: `Extract text content from PDF or EPUB documents on the device.

Requires poppler-utils (pdftotext) for PDFs. EPUBs are parsed natively.`,
	Example: `  remarkable read "My Notes"
  remarkable read "Book Title" --page 5`,
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

		// must be pdf or epub
		ft := doc.FileType
		if ft == "" {
			ft = "notebook"
		}
		if ft != "pdf" && ft != "epub" {
			err := fmt.Errorf("%q is a %s — read extracts text from pdf and epub only. Use 'remarkable export' for notebooks", doc.Name, ft)
			outputError(err)
			return err
		}

		// download source file
		rc, err := t.ReadFile(doc.ID, ft)
		if err != nil {
			outputError(err)
			return err
		}
		data, _ := io.ReadAll(rc)
		rc.Close()

		var text string
		var pages int

		switch ft {
		case "pdf":
			text, pages, err = extractPDF(data, readPage)
		case "epub":
			text, pages, err = extractEPUB(data)
		}
		if err != nil {
			outputError(err)
			return err
		}

		output(map[string]any{
			"id":    doc.ID,
			"name":  doc.Name,
			"text":  text,
			"pages": pages,
		})
		return nil
	},
}

// extractPDF writes to temp file, calls pdftotext
func extractPDF(data []byte, page int) (string, int, error) {
	// check pdftotext is available
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return "", 0, fmt.Errorf("install poppler-utils for PDF text extraction (brew install poppler)")
	}

	// write to temp file
	tmp, err := os.CreateTemp("", "remarkable-*.pdf")
	if err != nil {
		return "", 0, err
	}
	defer os.Remove(tmp.Name())
	tmp.Write(data)
	tmp.Close()

	// build pdftotext args
	args := []string{"-layout"}
	if page > 0 {
		args = append(args, "-f", fmt.Sprintf("%d", page), "-l", fmt.Sprintf("%d", page))
	}
	args = append(args, tmp.Name(), "-")

	out, err := exec.Command("pdftotext", args...).Output()
	if err != nil {
		return "", 0, fmt.Errorf("pdftotext failed: %w", err)
	}

	// count pages with pdfinfo
	pageCount := countPDFPages(tmp.Name())
	if page > 0 {
		pageCount = 1
	}

	return string(out), pageCount, nil
}

// countPDFPages uses pdfinfo to get page count
func countPDFPages(path string) int {
	out, err := exec.Command("pdfinfo", path).Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			var n int
			fmt.Sscanf(strings.TrimPrefix(line, "Pages:"), "%d", &n)
			return n
		}
	}
	return 0
}

// extractEPUB opens the zip and concatenates text from xhtml/html files
func extractEPUB(data []byte) (string, int, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", 0, fmt.Errorf("invalid epub: %w", err)
	}

	// collect html/xhtml files sorted by name
	var htmlFiles []*zip.File
	for _, f := range r.File {
		name := strings.ToLower(f.Name)
		if strings.HasSuffix(name, ".xhtml") || strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".htm") {
			htmlFiles = append(htmlFiles, f)
		}
	}
	sort.Slice(htmlFiles, func(i, j int) bool {
		return htmlFiles[i].Name < htmlFiles[j].Name
	})

	var parts []string
	for _, f := range htmlFiles {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		raw, _ := io.ReadAll(rc)
		rc.Close()

		text := stripHTML(string(raw))
		if text != "" {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, "\n\n"), len(htmlFiles), nil
}

// stripHTML removes tags and decodes common entities
func stripHTML(s string) string {
	// remove tags
	re := regexp.MustCompile("<[^>]*>")
	clean := re.ReplaceAllString(s, "")

	// decode entities
	clean = strings.ReplaceAll(clean, "&amp;", "&")
	clean = strings.ReplaceAll(clean, "&lt;", "<")
	clean = strings.ReplaceAll(clean, "&gt;", ">")
	clean = strings.ReplaceAll(clean, "&nbsp;", " ")
	clean = strings.ReplaceAll(clean, "&#39;", "'")
	clean = strings.ReplaceAll(clean, "&quot;", "\"")

	return strings.TrimSpace(clean)
}

func init() {
	readCmd.Flags().IntVar(&readPage, "page", 0, "extract single page (1-indexed, PDF only)")
	rootCmd.AddCommand(readCmd)
}
