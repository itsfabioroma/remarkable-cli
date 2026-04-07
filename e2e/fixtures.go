package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TinyPDF writes a minimal valid PDF to a temp file and returns its path.
// The file lasts for the duration of the test (cleaned up by t.TempDir).
func TinyPDF(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".pdf")
	if err := os.WriteFile(path, []byte(minimalPDF), 0600); err != nil {
		t.Fatalf("write fixture pdf: %v", err)
	}
	return path
}

// minimalPDF is a 1-page hand-built PDF (just enough for reMarkable to ingest).
const minimalPDF = `%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>
endobj
4 0 obj
<< /Length 44 >>
stream
BT /F1 24 Tf 100 700 Td (rmcli e2e test) Tj ET
endstream
endobj
5 0 obj
<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>
endobj
xref
0 6
0000000000 65535 f
0000000009 00000 n
0000000058 00000 n
0000000109 00000 n
0000000216 00000 n
0000000300 00000 n
trailer
<< /Size 6 /Root 1 0 R >>
startxref
365
%%EOF
`

// _ keeps fmt imported for callers that may extend fixtures
var _ = fmt.Sprintf
