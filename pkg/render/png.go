package render

import (
	"image"
	"image/color"
	"image/png"
	"io"
	"math"

	"github.com/itsfabioroma/remarkable-cli/pkg/encoding/rm"
	"github.com/itsfabioroma/remarkable-cli/pkg/model"
)

// PNGRenderer renders .rm strokes directly to PNG — no external tools needed
type PNGRenderer struct{}

// RenderPagePNG renders parsed blocks to a PNG image, handling any page height
func RenderPagePNG(w io.Writer, blocks []rm.Block) error {
	// extract lines
	var lines []rm.Line
	for _, b := range blocks {
		if b.Type == rm.BlockSceneLineItem {
			if data, ok := b.Data.(*rm.SceneLineData); ok && data != nil && len(data.Line.Points) > 0 {
				lines = append(lines, data.Line)
			}
		}
	}

	// compute bounding box
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

	if len(lines) == 0 {
		minX, minY, maxX, maxY = 0, 0, 1600, 2100
	}

	// add padding
	pad := float32(40)
	minX -= pad
	minY -= pad
	maxX += pad
	maxY += pad

	// scale to target width of 1600px, proportional height
	contentW := maxX - minX
	contentH := maxY - minY
	targetW := 1600
	scale := float64(targetW) / float64(contentW)
	targetH := int(float64(contentH) * scale)

	if targetH < 100 {
		targetH = 100
	}

	// create image
	img := image.NewRGBA(image.Rect(0, 0, targetW, targetH))

	// fill white
	for y := 0; y < targetH; y++ {
		for x := 0; x < targetW; x++ {
			img.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}

	// draw each stroke
	for _, line := range lines {
		if line.Tool.IsEraser() {
			continue
		}

		c := line.Color.RGBA()
		opacity := model.StrokeOpacity(line.Tool, 1.0, 1.0)
		strokeColor := color.RGBA{
			R: uint8(float64(c.R) * opacity),
			G: uint8(float64(c.G) * opacity),
			B: uint8(float64(c.B) * opacity),
			A: uint8(255 * opacity),
		}

		// draw line segments between consecutive points
		for i := 1; i < len(line.Points); i++ {
			p0 := line.Points[i-1]
			p1 := line.Points[i]

			// transform to image coordinates
			x0 := (float64(p0.X) - float64(minX)) * scale
			y0 := (float64(p0.Y) - float64(minY)) * scale
			x1 := (float64(p1.X) - float64(minX)) * scale
			y1 := (float64(p1.Y) - float64(minY)) * scale

			// stroke width
			pressure := float64(p1.Pressure) / 255.0
			baseWidth := float64(p1.Width) / 4.0 * line.ThicknessScale
			w := model.StrokeWidth(line.Tool, baseWidth, pressure, 0) * scale
			if w < 1.0 {
				w = 1.0
			}

			drawLine(img, x0, y0, x1, y1, w, strokeColor)
		}
	}

	return png.Encode(w, img)
}

// drawLine draws an anti-aliased thick line using Bresenham + thickness
func drawLine(img *image.RGBA, x0, y0, x1, y1, width float64, c color.RGBA) {
	dx := x1 - x0
	dy := y1 - y0
	dist := math.Sqrt(dx*dx + dy*dy)

	if dist < 0.5 {
		// single point
		plotThick(img, int(x0), int(y0), width, c)
		return
	}

	// step along the line
	steps := int(dist*2) + 1
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := x0 + dx*t
		y := y0 + dy*t
		plotThick(img, int(x), int(y), width, c)
	}
}

// plotThick draws a filled circle at (cx, cy) with given radius
func plotThick(img *image.RGBA, cx, cy int, width float64, c color.RGBA) {
	r := int(width/2) + 1
	bounds := img.Bounds()

	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				px := cx + dx
				py := cy + dy
				if px >= bounds.Min.X && px < bounds.Max.X && py >= bounds.Min.Y && py < bounds.Max.Y {
					// alpha blend
					existing := img.RGBAAt(px, py)
					blended := blendOver(c, existing)
					img.SetRGBA(px, py, blended)
				}
			}
		}
	}
}

func blendOver(src, dst color.RGBA) color.RGBA {
	if src.A == 255 {
		return src
	}
	if src.A == 0 {
		return dst
	}

	sa := float64(src.A) / 255.0
	da := float64(dst.A) / 255.0
	outA := sa + da*(1-sa)

	if outA == 0 {
		return color.RGBA{}
	}

	return color.RGBA{
		R: uint8((float64(src.R)*sa + float64(dst.R)*da*(1-sa)) / outA),
		G: uint8((float64(src.G)*sa + float64(dst.G)*da*(1-sa)) / outA),
		B: uint8((float64(src.B)*sa + float64(dst.B)*da*(1-sa)) / outA),
		A: uint8(outA * 255),
	}
}
