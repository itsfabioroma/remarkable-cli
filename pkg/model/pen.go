package model

import "image/color"

// PenType identifies the drawing tool
type PenType int

const (
	PenPaintbrush   PenType = 0
	PenPencil       PenType = 1
	PenBallpoint    PenType = 2
	PenMarker       PenType = 3
	PenFineliner    PenType = 4
	PenHighlighter  PenType = 5
	PenEraser       PenType = 6
	PenMechPencil   PenType = 7
	PenEraserArea   PenType = 8
	PenPaintbrush2  PenType = 12
	PenMechPencil2  PenType = 13
	PenPencil2      PenType = 14
	PenBallpoint2   PenType = 15
	PenMarker2      PenType = 16
	PenFineliner2   PenType = 17
	PenHighlighter2 PenType = 18
	PenCalligraphy  PenType = 21
	PenShader       PenType = 23
)

// Normalize returns the canonical pen type (de-duplicates firmware variants)
func (p PenType) Normalize() PenType {
	switch p {
	case PenPaintbrush2:
		return PenPaintbrush
	case PenPencil2:
		return PenPencil
	case PenBallpoint2:
		return PenBallpoint
	case PenMarker2:
		return PenMarker
	case PenFineliner2:
		return PenFineliner
	case PenHighlighter2:
		return PenHighlighter
	case PenMechPencil2:
		return PenMechPencil
	default:
		return p
	}
}

// IsEraser returns true for eraser-type pens
func (p PenType) IsEraser() bool {
	n := p.Normalize()
	return n == PenEraser || n == PenEraserArea
}

// PenColor identifies the stroke color
type PenColor int

const (
	ColorBlack       PenColor = 0
	ColorGray        PenColor = 1
	ColorWhite       PenColor = 2
	ColorYellow      PenColor = 3
	ColorGreen       PenColor = 4
	ColorPink        PenColor = 5
	ColorBlue        PenColor = 6
	ColorRed         PenColor = 7
	ColorGrayOverlap PenColor = 8
	ColorHighlight   PenColor = 9
	ColorGreen2      PenColor = 10
	ColorCyan        PenColor = 11
	ColorMagenta     PenColor = 12
	ColorYellow2     PenColor = 13
)

// RGBA returns the color as an RGBA value
func (c PenColor) RGBA() color.RGBA {
	switch c {
	case ColorBlack:
		return color.RGBA{0, 0, 0, 255}
	case ColorGray, ColorGrayOverlap:
		return color.RGBA{128, 128, 128, 255}
	case ColorWhite:
		return color.RGBA{255, 255, 255, 255}
	case ColorYellow, ColorYellow2:
		return color.RGBA{255, 233, 74, 255}
	case ColorGreen, ColorGreen2:
		return color.RGBA{0, 168, 0, 255}
	case ColorPink:
		return color.RGBA{255, 105, 180, 255}
	case ColorBlue:
		return color.RGBA{0, 0, 255, 255}
	case ColorRed:
		return color.RGBA{255, 0, 0, 255}
	case ColorHighlight:
		return color.RGBA{255, 233, 74, 100}
	case ColorCyan:
		return color.RGBA{0, 255, 255, 255}
	case ColorMagenta:
		return color.RGBA{255, 0, 255, 255}
	default:
		return color.RGBA{0, 0, 0, 255}
	}
}

// StrokeWidth computes the rendered width for a given pen at a specific pressure/speed
func StrokeWidth(pen PenType, baseWidth, pressure, speed float64) float64 {
	switch pen.Normalize() {
	case PenBallpoint:
		// pressure-sensitive width
		return baseWidth + (pressure-1)*baseWidth/2

	case PenMarker:
		// constant scale-down
		return baseWidth * 0.7

	case PenPencil:
		// always narrower
		return baseWidth - 0.42*baseWidth

	case PenMechPencil:
		return baseWidth / 1.5

	case PenPaintbrush:
		// pressure + speed sensitive
		return baseWidth*0.75 + (pressure-1)*baseWidth*0.5625

	case PenFineliner, PenCalligraphy:
		// use base width as-is
		return baseWidth

	case PenHighlighter:
		// fixed wide stroke
		return baseWidth

	default:
		return baseWidth
	}
}

// StrokeOpacity computes the rendered opacity for a given pen
func StrokeOpacity(pen PenType, pressure, speed float64) float64 {
	switch pen.Normalize() {
	case PenHighlighter:
		return 0.392

	case PenPencil, PenMechPencil:
		// lightened by pressure
		return pressure

	case PenPaintbrush:
		// pressure * speed factor
		pressMod := pressure * (2 - speed/75)
		if pressMod > 1 {
			pressMod = 1
		}
		return pressMod / 2

	default:
		return 1.0
	}
}
