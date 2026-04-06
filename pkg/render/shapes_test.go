package render

import (
	"testing"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
)

func TestMakePoint(t *testing.T) {
	p := makePoint(10, 20, 2.0)
	if p.X != 10 || p.Y != 20 {
		t.Fatalf("wrong position: got (%f, %f)", p.X, p.Y)
	}
	if p.Speed != 50 {
		t.Fatalf("expected speed 50, got %d", p.Speed)
	}
	if p.Width != 8 { // 2.0 * 4
		t.Fatalf("expected width 8, got %d", p.Width)
	}
	if p.Pressure != 200 {
		t.Fatalf("expected pressure 200, got %d", p.Pressure)
	}
}

func TestStrokeLine(t *testing.T) {
	line := StrokeLine(0, 0, 100, 100, model.PenFineliner, model.ColorBlack, 1.5)
	if len(line.Points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(line.Points))
	}
	if line.Tool != model.PenFineliner {
		t.Fatalf("wrong pen type")
	}
	if line.Color != model.ColorBlack {
		t.Fatalf("wrong color")
	}
}

func TestStrokeRect(t *testing.T) {
	lines := StrokeRect(10, 10, 100, 50, model.PenBallpoint, model.ColorRed, 2.0)
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(lines))
	}

	// each side has 2 points
	for i, l := range lines {
		if len(l.Points) != 2 {
			t.Fatalf("line %d: expected 2 points, got %d", i, len(l.Points))
		}
	}
}

func TestStrokeCircle(t *testing.T) {
	segments := 36
	line := StrokeCircle(100, 100, 50, segments, model.PenPencil, model.ColorBlue, 1.0)

	// segments+1 points (closed loop)
	if len(line.Points) != segments+1 {
		t.Fatalf("expected %d points, got %d", segments+1, len(line.Points))
	}

	// first and last point should be the same (closed)
	first := line.Points[0]
	last := line.Points[segments]
	if first.X != last.X || first.Y != last.Y {
		t.Fatalf("circle not closed: first=(%f,%f) last=(%f,%f)", first.X, first.Y, last.X, last.Y)
	}
}

func TestStrokeArrow(t *testing.T) {
	lines := StrokeArrow(0, 0, 100, 0, model.PenFineliner, model.ColorBlack, 1.0)

	// shaft + 2 arrowhead wings
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	// each line has 2 points
	for i, l := range lines {
		if len(l.Points) != 2 {
			t.Fatalf("line %d: expected 2 points, got %d", i, len(l.Points))
		}
	}
}
