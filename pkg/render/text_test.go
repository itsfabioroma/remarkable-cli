package render

import (
	"testing"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
)

func TestRenderTextHello(t *testing.T) {
	lines := RenderText("Hello", 100, 100, 3.0, model.PenFineliner, model.ColorBlack, 1.0)

	if len(lines) == 0 {
		t.Fatal("expected >0 lines for 'Hello'")
	}

	// count total points
	total := 0
	for _, l := range lines {
		total += len(l.Points)
	}
	if total < 5 {
		t.Errorf("expected reasonable point count, got %d", total)
	}
}

func TestRenderTextNewline(t *testing.T) {
	lines := RenderText("A\nB", 100, 100, 3.0, model.PenFineliner, model.ColorBlack, 1.0)

	if len(lines) == 0 {
		t.Fatal("expected >0 lines for 'A\\nB'")
	}

	// find max Y of A lines (before newline) and min Y of B lines (after newline)
	// A has 2 strokes, B has 2 strokes
	aMaxY := float32(0)
	for _, l := range lines[:2] {
		for _, p := range l.Points {
			if p.Y > aMaxY {
				aMaxY = p.Y
			}
		}
	}

	bMinY := float32(999999)
	for _, l := range lines[2:] {
		for _, p := range l.Points {
			if p.Y < bMinY {
				bMinY = p.Y
			}
		}
	}

	if bMinY <= aMaxY {
		t.Errorf("B lines should be below A lines: B minY=%f, A maxY=%f", bMinY, aMaxY)
	}
}
