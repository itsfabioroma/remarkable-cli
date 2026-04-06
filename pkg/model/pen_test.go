package model

import (
	"math"
	"testing"
)

func TestPenType_Normalize(t *testing.T) {
	tests := []struct {
		input    PenType
		expected PenType
	}{
		{PenBallpoint, PenBallpoint},
		{PenBallpoint2, PenBallpoint},
		{PenPencil, PenPencil},
		{PenPencil2, PenPencil},
		{PenHighlighter, PenHighlighter},
		{PenHighlighter2, PenHighlighter},
		{PenCalligraphy, PenCalligraphy},
	}

	for _, tt := range tests {
		got := tt.input.Normalize()
		if got != tt.expected {
			t.Errorf("PenType(%d).Normalize() = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestPenType_IsEraser(t *testing.T) {
	if !PenEraser.IsEraser() {
		t.Error("PenEraser should be eraser")
	}
	if !PenEraserArea.IsEraser() {
		t.Error("PenEraserArea should be eraser")
	}
	if PenBallpoint.IsEraser() {
		t.Error("PenBallpoint should not be eraser")
	}
}

func TestPenColor_RGBA(t *testing.T) {
	// black
	c := ColorBlack.RGBA()
	if c.R != 0 || c.G != 0 || c.B != 0 || c.A != 255 {
		t.Errorf("black: got %v", c)
	}

	// highlighter yellow
	c = ColorHighlight.RGBA()
	if c.A >= 255 {
		t.Error("highlight should be semi-transparent")
	}
}

func almostEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) < tolerance
}

func TestStrokeWidth_Ballpoint(t *testing.T) {
	// at pressure 1.0, width should equal base
	w := StrokeWidth(PenBallpoint, 2.0, 1.0, 0)
	if !almostEqual(w, 2.0, 0.01) {
		t.Errorf("ballpoint at pressure 1.0: expected 2.0, got %f", w)
	}

	// at pressure 2.0, width should be wider
	w = StrokeWidth(PenBallpoint, 2.0, 2.0, 0)
	if !almostEqual(w, 3.0, 0.01) {
		t.Errorf("ballpoint at pressure 2.0: expected 3.0, got %f", w)
	}
}

func TestStrokeWidth_Marker(t *testing.T) {
	w := StrokeWidth(PenMarker, 10.0, 1.0, 0)
	if !almostEqual(w, 7.0, 0.01) {
		t.Errorf("marker: expected 7.0, got %f", w)
	}
}

func TestStrokeWidth_Pencil(t *testing.T) {
	w := StrokeWidth(PenPencil, 10.0, 1.0, 0)
	if !almostEqual(w, 5.8, 0.01) {
		t.Errorf("pencil: expected 5.8, got %f", w)
	}
}

func TestStrokeWidth_Fineliner(t *testing.T) {
	// fineliner uses base width as-is
	w := StrokeWidth(PenFineliner, 3.0, 1.0, 0)
	if !almostEqual(w, 3.0, 0.01) {
		t.Errorf("fineliner: expected 3.0, got %f", w)
	}

	// also works for firmware variant
	w = StrokeWidth(PenFineliner2, 3.0, 1.0, 0)
	if !almostEqual(w, 3.0, 0.01) {
		t.Errorf("fineliner2: expected 3.0, got %f", w)
	}
}

func TestStrokeOpacity_Highlighter(t *testing.T) {
	o := StrokeOpacity(PenHighlighter, 1.0, 0)
	if !almostEqual(o, 0.392, 0.001) {
		t.Errorf("highlighter opacity: expected 0.392, got %f", o)
	}
}

func TestStrokeOpacity_Default(t *testing.T) {
	// most pens are fully opaque
	o := StrokeOpacity(PenBallpoint, 1.0, 0)
	if o != 1.0 {
		t.Errorf("ballpoint opacity: expected 1.0, got %f", o)
	}
}
