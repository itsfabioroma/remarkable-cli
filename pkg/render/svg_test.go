package render

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabioroma/remarkable-cli/pkg/encoding/rm"
)

func TestSVGRenderer_RenderFixture(t *testing.T) {
	// render the densest quick sheet page
	fixtures := []struct {
		name string
		path string
	}{
		{"quicksheet-dense", "/Users/fabioroma/Code/remarkable-cli/testdata/fixtures/quicksheets/ddc256b6-c87e-49df-9a90-5cde3ead67d5.rm"},
		{"quicksheet-medium", "/Users/fabioroma/Code/remarkable-cli/testdata/fixtures/quicksheets/95eb0619-4e60-4308-90f2-bc9f0c6dc176.rm"},
		{"notebook", "/Users/fabioroma/Code/remarkable-cli/testdata/fixtures/notebook/bd90a8ad-bf38-44e8-ba50-f17b051194d7.rm"},
	}

	r := NewSVGRenderer()

	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			data, err := os.ReadFile(f.path)
			if err != nil {
				t.Skip("fixture not found")
			}

			blocks, err := rm.ParseBlocks(data)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			var buf bytes.Buffer
			if err := r.RenderPage(&buf, blocks); err != nil {
				t.Fatalf("render error: %v", err)
			}

			svg := buf.String()

			// basic SVG validity checks
			if !strings.Contains(svg, "<svg") {
				t.Error("missing <svg> element")
			}
			if !strings.Contains(svg, "</svg>") {
				t.Error("missing </svg> closing tag")
			}

			// should have path elements (strokes)
			pathCount := strings.Count(svg, "<path")
			t.Logf("rendered %d paths, %d bytes SVG", pathCount, len(svg))

			if pathCount == 0 {
				t.Error("no paths rendered (expected strokes)")
			}

			// write to /tmp for visual inspection
			outPath := filepath.Join(os.TempDir(), "remarkable-"+f.name+".svg")
			os.WriteFile(outPath, buf.Bytes(), 0644)
			t.Logf("SVG written to %s", outPath)
		})
	}
}
