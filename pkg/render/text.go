package render

import (
	"strings"

	"github.com/itsfabioroma/remarkable-cli/pkg/encoding/rm"
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
)

// page dimensions (centered coordinate system, ~-700 to +700 X, 0 to 1800 Y)
const (
	pageMinX  float32 = -600
	pageMaxX  float32 = 600
	pageWidth float32 = 1200
	charGap   float32 = 3 // extra spacing between characters
)

// RenderText converts text to pen strokes, auto-scaling to fit the page.
// startY is where to begin vertically. Returns the strokes and the bottomY of content.
func RenderText(text string, startX, startY, scale float32, pen model.PenType, color model.PenColor, thickness float64) []rm.Line {
	return renderTextAt(text, startX, startY, scale, pen, color, thickness)
}

// RenderTextAutoScale renders text with automatic scale based on text length.
// Short text gets large scale, long text gets smaller but still readable, and wraps.
func RenderTextAutoScale(text string, startY float32, pen model.PenType, color model.PenColor, thickness float64) ([]rm.Line, float32) {
	// pick scale based on total text length (not line width — wrapping handles overflow)
	totalChars := len([]rune(text))
	scale := float32(5.0)

	switch {
	case totalChars < 30:
		scale = 6.0 // large for short text
	case totalChars < 80:
		scale = 4.5 // medium
	case totalChars < 200:
		scale = 3.5 // readable with wrapping
	case totalChars < 500:
		scale = 2.5 // small but legible
	default:
		scale = 2.0 // dense
	}

	lines := renderTextWrapped(text, pageMinX+30, startY, scale, pen, color, thickness)

	// find bottom Y
	bottomY := startY
	for _, l := range lines {
		for _, p := range l.Points {
			if p.Y > bottomY {
				bottomY = p.Y
			}
		}
	}

	return lines, bottomY
}

// estimateLineWidth estimates the width of a line in font units (before scaling)
func estimateLineWidth(line string) float32 {
	w := float32(0)
	for _, r := range line {
		if cd, ok := model.StrokeFont[r]; ok {
			w += cd.Width + charGap
		} else {
			w += 8 + charGap // unknown char
		}
	}
	return w
}

// renderTextWrapped renders with word wrapping at page boundaries
func renderTextWrapped(text string, x, y, scale float32, pen model.PenType, color model.PenColor, thickness float64) []rm.Line {
	var allLines []rm.Line
	lineHeight := (24 + 4) * scale // font height + padding
	curY := y

	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			curY += lineHeight
			continue
		}

		words := strings.Fields(paragraph)
		curX := x

		for _, word := range words {
			// check if word fits on current line
			wordWidth := estimateLineWidth(word) * scale
			if curX+wordWidth > pageMaxX-30 && curX > x+10 {
				// wrap to next line
				curX = x
				curY += lineHeight
			}

			// render each character
			for _, r := range word {
				cd, ok := model.StrokeFont[r]
				if !ok {
					curX += (8 + charGap) * scale
					continue
				}

				for _, stroke := range cd.Strokes {
					pts := make([]rm.Point, len(stroke.Points))
					for i, p := range stroke.Points {
						pts[i] = rm.Point{
							X:         curX + p.X*scale,
							Y:         curY + p.Y*scale,
							Speed:     0,
							Width:     16,
							Direction: 0,
							Pressure:  20,
						}
					}
					allLines = append(allLines, rm.Line{
						Tool:           pen,
						Color:          color,
						ThicknessScale: thickness,
						Points:         pts,
					})
				}

				curX += (cd.Width + charGap) * scale
			}

			// space after word
			curX += (6 + charGap) * scale
		}

		curY += lineHeight
	}

	return allLines
}

// renderTextAt renders text at exact position (no wrapping, no auto-scale)
func renderTextAt(text string, x, y, scale float32, pen model.PenType, color model.PenColor, thickness float64) []rm.Line {
	var lines []rm.Line
	originX := x
	curX := x
	curY := y
	lineHeight := (24 + 4) * scale

	for _, r := range text {
		if r == '\n' {
			curX = originX
			curY += lineHeight
			continue
		}

		charDef, ok := model.StrokeFont[r]
		if !ok {
			curX += (8 + charGap) * scale
			continue
		}

		for _, stroke := range charDef.Strokes {
			pts := make([]rm.Point, len(stroke.Points))
			for i, p := range stroke.Points {
				pts[i] = rm.Point{
					X:         curX + p.X*scale,
					Y:         curY + p.Y*scale,
					Speed:     0,
					Width:     16,
					Direction: 0,
					Pressure:  20,
				}
			}
			lines = append(lines, rm.Line{
				Tool:           pen,
				Color:          color,
				ThicknessScale: thickness,
				Points:         pts,
			})
		}

		curX += (charDef.Width + charGap) * scale
	}

	return lines
}
