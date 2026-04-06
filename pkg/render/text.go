package render

import (
	"github.com/itsfabioroma/remarkable-cli/pkg/encoding/rm"
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
)

// RenderText converts a text string into pen strokes positioned at (x, y).
// scale multiplies the base font size (~20 units). scale=1.0 is small, scale=3.0 is readable.
// Returns one rm.Line per character stroke.
func RenderText(text string, x, y, scale float32, pen model.PenType, color model.PenColor, thickness float64) []rm.Line {
	var lines []rm.Line
	originX := x
	curX := x
	curY := y

	// line height: font is ~20 units + 4 units spacing
	lineHeight := 24 * scale

	for _, r := range text {

		// newline: carriage return
		if r == '\n' {
			curX = originX
			curY += lineHeight
			continue
		}

		// look up glyph
		charDef, ok := model.StrokeFont[r]
		if !ok {
			// unknown rune: skip with a small advance
			curX += 6 * scale
			continue
		}

		// convert each stroke to an rm.Line
		for _, stroke := range charDef.Strokes {
			pts := make([]rm.Point, len(stroke.Points))
			for i, p := range stroke.Points {
				pts[i] = makePoint(
					curX+p.X*scale,
					curY+p.Y*scale,
					thickness,
				)
			}
			lines = append(lines, rm.Line{
				Tool:           pen,
				Color:          color,
				ThicknessScale: thickness,
				Points:         pts,
			})
		}

		// advance cursor
		curX += charDef.Width * scale
	}

	return lines
}
