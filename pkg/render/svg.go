package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/itsfabioroma/remarkable-cli/pkg/encoding/rm"
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
)

// screen dimensions
const (
	RMWidth  = 1404
	RMHeight = 1872

	PPWidth  = 1632
	PPHeight = 2154
)

// SVGRenderer renders parsed .rm strokes to SVG
type SVGRenderer struct {
	Width  int
	Height int
}

// NewSVGRenderer creates a renderer with Paper Pro dimensions by default
func NewSVGRenderer() *SVGRenderer {
	return &SVGRenderer{Width: PPWidth, Height: PPHeight}
}

// RenderPage writes an SVG document from parsed blocks
func (s *SVGRenderer) RenderPage(w io.Writer, blocks []rm.Block) error {
	// extract lines from blocks
	var lines []rm.Line
	for _, b := range blocks {
		if b.Type == rm.BlockSceneLineItem {
			if data, ok := b.Data.(*rm.SceneLineData); ok && data != nil {
				if len(data.Line.Points) > 0 {
					lines = append(lines, data.Line)
				}
			}
		}
	}

	// compute bounding box from actual points
	minX, minY := float32(1e9), float32(1e9)
	maxX, maxY := float32(-1e9), float32(-1e9)
	for _, line := range lines {
		for _, pt := range line.Points {
			if pt.X < minX { minX = pt.X }
			if pt.Y < minY { minY = pt.Y }
			if pt.X > maxX { maxX = pt.X }
			if pt.Y > maxY { maxY = pt.Y }
		}
	}

	// add padding
	pad := float32(20)
	minX -= pad
	minY -= pad
	maxX += pad
	maxY += pad

	// use device dimensions if no strokes or bbox is within screen
	viewW := maxX - minX
	viewH := maxY - minY
	if len(lines) == 0 {
		viewW = float32(s.Width)
		viewH = float32(s.Height)
		minX = 0
		minY = 0
	}

	// SVG header
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="%.1f %.1f %.1f %.1f" width="%.0f" height="%.0f">
<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="white"/>
`,
		minX, minY, viewW, viewH,
		viewW, viewH,
		minX, minY, viewW, viewH)

	// render each stroke
	for _, line := range lines {
		if line.Tool.IsEraser() {
			continue
		}

		s.renderStroke(w, line)
	}

	fmt.Fprintf(w, "</svg>\n")
	return nil
}

func (s *SVGRenderer) renderStroke(w io.Writer, line rm.Line) {
	if len(line.Points) < 2 {
		return
	}

	// pen color
	c := line.Color.RGBA()
	colorStr := fmt.Sprintf("rgb(%d,%d,%d)", c.R, c.G, c.B)

	// highlighter gets special treatment
	pen := line.Tool.Normalize()
	opacity := model.StrokeOpacity(line.Tool, 1.0, 1.0)

	// build path data
	var pathParts []string
	for i, pt := range line.Points {
		// compute width for this segment
		pressure := float64(pt.Pressure) / 255.0
		speed := float64(pt.Speed)
		baseWidth := float64(pt.Width) / 4.0 * line.ThicknessScale

		strokeWidth := model.StrokeWidth(line.Tool, baseWidth, pressure, speed)
		if strokeWidth < 0.5 {
			strokeWidth = 0.5
		}

		if i == 0 {
			pathParts = append(pathParts, fmt.Sprintf("M %.2f %.2f", pt.X, pt.Y))
		} else {
			pathParts = append(pathParts, fmt.Sprintf("L %.2f %.2f", pt.X, pt.Y))
		}

		// for variable width, we use per-segment rendering
		_ = strokeWidth
	}

	// average stroke width for the whole path
	avgWidth := averageWidth(line)

	// line cap and join
	lineCap := "round"
	lineJoin := "round"
	if pen == model.PenHighlighter {
		lineCap = "square"
	}

	// write path element
	fmt.Fprintf(w, `  <path d="%s" fill="none" stroke="%s" stroke-width="%.2f" stroke-linecap="%s" stroke-linejoin="%s"`,
		strings.Join(pathParts, " "), colorStr, avgWidth, lineCap, lineJoin)

	if opacity < 1.0 {
		fmt.Fprintf(w, ` opacity="%.3f"`, opacity)
	}

	fmt.Fprintf(w, "/>\n")
}

// averageWidth computes the mean rendered width across all points in a stroke
func averageWidth(line rm.Line) float64 {
	if len(line.Points) == 0 {
		return 1.0
	}

	total := 0.0
	for _, pt := range line.Points {
		pressure := float64(pt.Pressure) / 255.0
		speed := float64(pt.Speed)
		baseWidth := float64(pt.Width) / 4.0 * line.ThicknessScale

		w := model.StrokeWidth(line.Tool, baseWidth, pressure, speed)
		if w < 0.5 {
			w = 0.5
		}
		total += w
	}

	return total / float64(len(line.Points))
}
