package model

// CharStroke is a single pen stroke (polyline) within a character
type CharStroke struct {
	Points []struct{ X, Y float32 }
}

// CharDef defines a character's geometry and width
type CharDef struct {
	Width   float32
	Strokes []CharStroke
}

// StrokeFont maps runes to stroke definitions (~20 units tall, print-style)
var StrokeFont = map[rune]CharDef{

	// === Uppercase letters ===

	'A': {Width: 14, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 20}, {7, 0}, {14, 20}}},
		{Points: []struct{ X, Y float32 }{{3, 12}, {11, 12}}},
	}},
	'B': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 20}, {0, 0}, {8, 0}, {12, 3}, {12, 7}, {8, 10}, {0, 10}}},
		{Points: []struct{ X, Y float32 }{{0, 10}, {8, 10}, {12, 13}, {12, 17}, {8, 20}, {0, 20}}},
	}},
	'C': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{12, 3}, {9, 0}, {3, 0}, {0, 3}, {0, 17}, {3, 20}, {9, 20}, {12, 17}}},
	}},
	'D': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {7, 0}, {12, 5}, {12, 15}, {7, 20}, {0, 20}, {0, 0}}},
	}},
	'E': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{12, 0}, {0, 0}, {0, 20}, {12, 20}}},
		{Points: []struct{ X, Y float32 }{{0, 10}, {9, 10}}},
	}},
	'F': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{12, 0}, {0, 0}, {0, 20}}},
		{Points: []struct{ X, Y float32 }{{0, 10}, {9, 10}}},
	}},
	'G': {Width: 14, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{14, 3}, {11, 0}, {3, 0}, {0, 3}, {0, 17}, {3, 20}, {11, 20}, {14, 17}, {14, 10}, {8, 10}}},
	}},
	'H': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {0, 20}}},
		{Points: []struct{ X, Y float32 }{{12, 0}, {12, 20}}},
		{Points: []struct{ X, Y float32 }{{0, 10}, {12, 10}}},
	}},
	'I': {Width: 8, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{1, 0}, {7, 0}}},
		{Points: []struct{ X, Y float32 }{{4, 0}, {4, 20}}},
		{Points: []struct{ X, Y float32 }{{1, 20}, {7, 20}}},
	}},
	'J': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{10, 0}, {10, 17}, {7, 20}, {3, 20}, {0, 17}}},
	}},
	'K': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {0, 20}}},
		{Points: []struct{ X, Y float32 }{{12, 0}, {0, 10}, {12, 20}}},
	}},
	'L': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {0, 20}, {12, 20}}},
	}},
	'M': {Width: 14, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 20}, {0, 0}, {7, 12}, {14, 0}, {14, 20}}},
	}},
	'N': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 20}, {0, 0}, {12, 20}, {12, 0}}},
	}},
	'O': {Width: 14, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{3, 0}, {11, 0}, {14, 3}, {14, 17}, {11, 20}, {3, 20}, {0, 17}, {0, 3}, {3, 0}}},
	}},
	'P': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 20}, {0, 0}, {8, 0}, {12, 3}, {12, 7}, {8, 10}, {0, 10}}},
	}},
	'Q': {Width: 14, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{3, 0}, {11, 0}, {14, 3}, {14, 17}, {11, 20}, {3, 20}, {0, 17}, {0, 3}, {3, 0}}},
		{Points: []struct{ X, Y float32 }{{10, 16}, {14, 20}}},
	}},
	'R': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 20}, {0, 0}, {8, 0}, {12, 3}, {12, 7}, {8, 10}, {0, 10}}},
		{Points: []struct{ X, Y float32 }{{8, 10}, {12, 20}}},
	}},
	'S': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{12, 3}, {9, 0}, {3, 0}, {0, 3}, {0, 7}, {3, 10}, {9, 10}, {12, 13}, {12, 17}, {9, 20}, {3, 20}, {0, 17}}},
	}},
	'T': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {12, 0}}},
		{Points: []struct{ X, Y float32 }{{6, 0}, {6, 20}}},
	}},
	'U': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {0, 17}, {3, 20}, {9, 20}, {12, 17}, {12, 0}}},
	}},
	'V': {Width: 14, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {7, 20}, {14, 0}}},
	}},
	'W': {Width: 14, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {3, 20}, {7, 8}, {11, 20}, {14, 0}}},
	}},
	'X': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {12, 20}}},
		{Points: []struct{ X, Y float32 }{{12, 0}, {0, 20}}},
	}},
	'Y': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {6, 10}, {12, 0}}},
		{Points: []struct{ X, Y float32 }{{6, 10}, {6, 20}}},
	}},
	'Z': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {12, 0}, {0, 20}, {12, 20}}},
	}},

	// === Lowercase letters ===

	'a': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{10, 8}, {7, 6}, {3, 6}, {0, 9}, {0, 17}, {3, 20}, {7, 20}, {10, 17}}},
		{Points: []struct{ X, Y float32 }{{10, 6}, {10, 20}}},
	}},
	'b': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {0, 20}}},
		{Points: []struct{ X, Y float32 }{{0, 8}, {3, 6}, {7, 6}, {10, 9}, {10, 17}, {7, 20}, {3, 20}, {0, 17}}},
	}},
	'c': {Width: 9, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{9, 8}, {6, 6}, {3, 6}, {0, 9}, {0, 17}, {3, 20}, {6, 20}, {9, 18}}},
	}},
	'd': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{10, 0}, {10, 20}}},
		{Points: []struct{ X, Y float32 }{{10, 8}, {7, 6}, {3, 6}, {0, 9}, {0, 17}, {3, 20}, {7, 20}, {10, 17}}},
	}},
	'e': {Width: 9, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 13}, {9, 13}, {9, 9}, {6, 6}, {3, 6}, {0, 9}, {0, 17}, {3, 20}, {6, 20}, {9, 18}}},
	}},
	'f': {Width: 8, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{8, 3}, {6, 0}, {3, 0}, {2, 3}, {2, 20}}},
		{Points: []struct{ X, Y float32 }{{0, 8}, {6, 8}}},
	}},
	'g': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{10, 8}, {7, 6}, {3, 6}, {0, 9}, {0, 17}, {3, 20}, {7, 20}, {10, 17}}},
		{Points: []struct{ X, Y float32 }{{10, 6}, {10, 24}, {7, 27}, {3, 27}}},
	}},
	'h': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {0, 20}}},
		{Points: []struct{ X, Y float32 }{{0, 9}, {3, 6}, {7, 6}, {10, 9}, {10, 20}}},
	}},
	'i': {Width: 4, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{2, 0}, {2, 2}}},
		{Points: []struct{ X, Y float32 }{{2, 6}, {2, 20}}},
	}},
	'j': {Width: 6, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{4, 0}, {4, 2}}},
		{Points: []struct{ X, Y float32 }{{4, 6}, {4, 24}, {2, 27}, {0, 27}}},
	}},
	'k': {Width: 9, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {0, 20}}},
		{Points: []struct{ X, Y float32 }{{9, 6}, {0, 14}, {9, 20}}},
	}},
	'l': {Width: 4, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{2, 0}, {2, 20}}},
	}},
	'm': {Width: 14, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 6}, {0, 20}}},
		{Points: []struct{ X, Y float32 }{{0, 9}, {3, 6}, {5, 6}, {7, 9}, {7, 20}}},
		{Points: []struct{ X, Y float32 }{{7, 9}, {10, 6}, {12, 6}, {14, 9}, {14, 20}}},
	}},
	'n': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 6}, {0, 20}}},
		{Points: []struct{ X, Y float32 }{{0, 9}, {3, 6}, {7, 6}, {10, 9}, {10, 20}}},
	}},
	'o': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{3, 6}, {7, 6}, {10, 9}, {10, 17}, {7, 20}, {3, 20}, {0, 17}, {0, 9}, {3, 6}}},
	}},
	'p': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 6}, {0, 27}}},
		{Points: []struct{ X, Y float32 }{{0, 8}, {3, 6}, {7, 6}, {10, 9}, {10, 17}, {7, 20}, {3, 20}, {0, 17}}},
	}},
	'q': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{10, 6}, {10, 27}}},
		{Points: []struct{ X, Y float32 }{{10, 8}, {7, 6}, {3, 6}, {0, 9}, {0, 17}, {3, 20}, {7, 20}, {10, 17}}},
	}},
	'r': {Width: 8, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 6}, {0, 20}}},
		{Points: []struct{ X, Y float32 }{{0, 10}, {3, 6}, {6, 6}, {8, 8}}},
	}},
	's': {Width: 9, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{9, 8}, {6, 6}, {3, 6}, {0, 8}, {0, 11}, {3, 13}, {6, 13}, {9, 15}, {9, 18}, {6, 20}, {3, 20}, {0, 18}}},
	}},
	't': {Width: 8, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{3, 0}, {3, 17}, {5, 20}, {8, 20}}},
		{Points: []struct{ X, Y float32 }{{0, 6}, {7, 6}}},
	}},
	'u': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 6}, {0, 17}, {3, 20}, {7, 20}, {10, 17}}},
		{Points: []struct{ X, Y float32 }{{10, 6}, {10, 20}}},
	}},
	'v': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 6}, {5, 20}, {10, 6}}},
	}},
	'w': {Width: 14, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 6}, {3, 20}, {7, 10}, {11, 20}, {14, 6}}},
	}},
	'x': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 6}, {10, 20}}},
		{Points: []struct{ X, Y float32 }{{10, 6}, {0, 20}}},
	}},
	'y': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 6}, {5, 14}}},
		{Points: []struct{ X, Y float32 }{{10, 6}, {3, 27}}},
	}},
	'z': {Width: 9, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 6}, {9, 6}, {0, 20}, {9, 20}}},
	}},

	// === Digits ===

	'0': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{3, 0}, {9, 0}, {12, 3}, {12, 17}, {9, 20}, {3, 20}, {0, 17}, {0, 3}, {3, 0}}},
	}},
	'1': {Width: 8, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{2, 4}, {5, 0}, {5, 20}}},
		{Points: []struct{ X, Y float32 }{{2, 20}, {8, 20}}},
	}},
	'2': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 3}, {3, 0}, {9, 0}, {12, 3}, {12, 7}, {0, 20}, {12, 20}}},
	}},
	'3': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 3}, {3, 0}, {9, 0}, {12, 3}, {12, 7}, {9, 10}, {4, 10}}},
		{Points: []struct{ X, Y float32 }{{9, 10}, {12, 13}, {12, 17}, {9, 20}, {3, 20}, {0, 17}}},
	}},
	'4': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{9, 20}, {9, 0}, {0, 14}, {12, 14}}},
	}},
	'5': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{12, 0}, {0, 0}, {0, 10}, {9, 10}, {12, 13}, {12, 17}, {9, 20}, {3, 20}, {0, 17}}},
	}},
	'6': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{9, 0}, {3, 0}, {0, 3}, {0, 17}, {3, 20}, {9, 20}, {12, 17}, {12, 13}, {9, 10}, {0, 10}}},
	}},
	'7': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 0}, {12, 0}, {4, 20}}},
	}},
	'8': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{3, 0}, {9, 0}, {12, 3}, {12, 7}, {9, 10}, {3, 10}, {0, 7}, {0, 3}, {3, 0}}},
		{Points: []struct{ X, Y float32 }{{3, 10}, {0, 13}, {0, 17}, {3, 20}, {9, 20}, {12, 17}, {12, 13}, {9, 10}}},
	}},
	'9': {Width: 12, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{12, 10}, {3, 10}, {0, 7}, {0, 3}, {3, 0}, {9, 0}, {12, 3}, {12, 17}, {9, 20}, {3, 20}}},
	}},

	// === Punctuation ===

	' ': {Width: 6, Strokes: nil},

	'.': {Width: 4, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{2, 19}, {2, 20}}},
	}},
	',': {Width: 4, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{2, 19}, {1, 23}}},
	}},
	':': {Width: 4, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{2, 7}, {2, 8}}},
		{Points: []struct{ X, Y float32 }{{2, 19}, {2, 20}}},
	}},
	';': {Width: 4, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{2, 7}, {2, 8}}},
		{Points: []struct{ X, Y float32 }{{2, 19}, {1, 23}}},
	}},
	'!': {Width: 4, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{2, 0}, {2, 14}}},
		{Points: []struct{ X, Y float32 }{{2, 19}, {2, 20}}},
	}},
	'?': {Width: 10, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{0, 3}, {3, 0}, {7, 0}, {10, 3}, {10, 7}, {5, 12}, {5, 14}}},
		{Points: []struct{ X, Y float32 }{{5, 19}, {5, 20}}},
	}},
	'-': {Width: 8, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{1, 10}, {7, 10}}},
	}},
	'(': {Width: 6, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{5, 0}, {2, 5}, {2, 15}, {5, 20}}},
	}},
	')': {Width: 6, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{1, 0}, {4, 5}, {4, 15}, {1, 20}}},
	}},
	'/': {Width: 8, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{8, 0}, {0, 20}}},
	}},
	'\'': {Width: 4, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{2, 0}, {2, 5}}},
	}},
	'"': {Width: 8, Strokes: []CharStroke{
		{Points: []struct{ X, Y float32 }{{2, 0}, {2, 5}}},
		{Points: []struct{ X, Y float32 }{{6, 0}, {6, 5}}},
	}},
}
