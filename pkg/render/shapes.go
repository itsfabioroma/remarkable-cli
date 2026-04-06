package render

import (
	"math"

	"github.com/itsfabioroma/remarkable-cli/pkg/encoding/rm"
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
)

// makePoint creates a point with standard pressure/speed defaults
func makePoint(x, y float32, width float64) rm.Point {
	return rm.Point{
		X:         x,
		Y:         y,
		Speed:     50,
		Width:     uint16(width * 4),
		Direction: 0,
		Pressure:  200,
	}
}

// StrokeLine creates a single line between two points
func StrokeLine(x1, y1, x2, y2 float32, pen model.PenType, color model.PenColor, thickness float64) rm.Line {
	return rm.Line{
		Tool:           pen,
		Color:          color,
		ThicknessScale: thickness,
		Points: []rm.Point{
			makePoint(x1, y1, thickness),
			makePoint(x2, y2, thickness),
		},
	}
}

// StrokeRect creates four lines forming a rectangle
func StrokeRect(x, y, w, h float32, pen model.PenType, color model.PenColor, thickness float64) []rm.Line {
	return []rm.Line{
		StrokeLine(x, y, x+w, y, pen, color, thickness),         // top
		StrokeLine(x+w, y, x+w, y+h, pen, color, thickness),     // right
		StrokeLine(x+w, y+h, x, y+h, pen, color, thickness),     // bottom
		StrokeLine(x, y+h, x, y, pen, color, thickness),         // left
	}
}

// StrokeCircle creates a single line with points around the circumference
func StrokeCircle(cx, cy, radius float32, segments int, pen model.PenType, color model.PenColor, thickness float64) rm.Line {
	points := make([]rm.Point, segments+1)
	for i := 0; i <= segments; i++ {
		angle := 2 * math.Pi * float64(i) / float64(segments)
		px := cx + radius*float32(math.Cos(angle))
		py := cy + radius*float32(math.Sin(angle))
		points[i] = makePoint(px, py, thickness)
	}

	return rm.Line{
		Tool:           pen,
		Color:          color,
		ThicknessScale: thickness,
		Points:         points,
	}
}

// StrokeArrow creates a shaft line plus two diagonal arrowhead lines
func StrokeArrow(x1, y1, x2, y2 float32, pen model.PenType, color model.PenColor, thickness float64) []rm.Line {

	// shaft
	shaft := StrokeLine(x1, y1, x2, y2, pen, color, thickness)

	// arrowhead: two short diagonals at the endpoint
	dx := float64(x2 - x1)
	dy := float64(y2 - y1)
	length := math.Sqrt(dx*dx + dy*dy)
	if length == 0 {
		return []rm.Line{shaft}
	}

	// unit vector along shaft
	ux := dx / length
	uy := dy / length

	// arrowhead size = 10% of shaft length, min 5px
	headLen := length * 0.1
	if headLen < 5 {
		headLen = 5
	}

	// two wings at +/- 30 degrees from reversed shaft direction
	cos30 := math.Cos(math.Pi / 6)
	sin30 := math.Sin(math.Pi / 6)

	// left wing
	lx := float32(float64(x2) - headLen*(ux*cos30+uy*sin30))
	ly := float32(float64(y2) - headLen*(-ux*sin30+uy*cos30))
	left := StrokeLine(x2, y2, lx, ly, pen, color, thickness)

	// right wing
	rx := float32(float64(x2) - headLen*(ux*cos30-uy*sin30))
	ry := float32(float64(y2) - headLen*(ux*sin30+uy*cos30))
	right := StrokeLine(x2, y2, rx, ry, pen, color, thickness)

	return []rm.Line{shaft, left, right}
}
