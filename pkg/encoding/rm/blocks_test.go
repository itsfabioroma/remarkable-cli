package rm

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// testFixtures returns all .rm files in testdata/fixtures
func testFixtures(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..", "..", "testdata", "fixtures")

	var files []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if filepath.Ext(path) == ".rm" {
			files = append(files, path)
		}
		return nil
	})

	if len(files) == 0 {
		t.Skip("no .rm fixtures found in testdata/fixtures")
	}

	return files
}

func TestParseBlocks_AllFixtures(t *testing.T) {
	for _, fixture := range testFixtures(t) {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			data, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("cannot read fixture: %v", err)
			}

			blocks, err := ParseBlocks(data)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			if len(blocks) == 0 {
				t.Fatal("no blocks parsed")
			}

			// count block types
			counts := make(map[BlockType]int)
			for _, b := range blocks {
				counts[b.Type]++
			}

			t.Logf("parsed %d blocks: %v", len(blocks), counts)

			// every file should have at least a MigrationInfo or AuthorIds block
			hasMeta := counts[BlockMigrationInfo] > 0 || counts[BlockAuthorIds] > 0
			if !hasMeta {
				t.Error("expected at least one metadata block (MigrationInfo or AuthorIds)")
			}
		})
	}
}

func TestParseBlocks_ExtractLines(t *testing.T) {
	for _, fixture := range testFixtures(t) {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			data, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("cannot read fixture: %v", err)
			}

			blocks, err := ParseBlocks(data)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			// extract line blocks
			var lines []Line
			for _, b := range blocks {
				if b.Type == BlockSceneLineItem {
					if lineData, ok := b.Data.(*SceneLineData); ok && lineData != nil {
						lines = append(lines, lineData.Line)
					}
				}
			}

			// count total points
			totalPoints := 0
			for _, line := range lines {
				totalPoints += len(line.Points)
			}

			t.Logf("found %d strokes, %d total points", len(lines), totalPoints)

			// validate points have reasonable coordinates
			for i, line := range lines {
				for j, pt := range line.Points {
					// reMarkable screen coords: ~0-1632 x, ~0-2154 y (Paper Pro)
					// but can be negative or larger for scrolled pages
					if pt.X < -5000 || pt.X > 5000 || pt.Y < -5000 || pt.Y > 5000 {
						t.Errorf("line %d point %d has suspicious coords: (%.1f, %.1f)",
							i, j, pt.X, pt.Y)
						break
					}
				}
			}
		})
	}
}

func TestParseBlocks_AuthorIds(t *testing.T) {
	for _, fixture := range testFixtures(t) {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			data, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("cannot read fixture: %v", err)
			}

			blocks, err := ParseBlocks(data)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			for _, b := range blocks {
				if b.Type == BlockAuthorIds {
					if authors, ok := b.Data.(*AuthorIdsData); ok && authors != nil {
						t.Logf("author map: %v", authors.Authors)
					}
				}
			}
		})
	}
}

func TestParseBlocks_ExtractGlyphsFromSyntheticFixture(t *testing.T) {
	const fixtureBase64 = "cmVNYXJrYWJsZSAubGluZXMgZmlsZSwgdmVyc2lvbj02ICAgICAgICAgIBkAAAAAAQEJAQwTAAAAEBEREREREREREREREREREREBAAcAAAAAAQEAHwEBIQExABkAAAAAAAEKFAEAAAAkAAAAADQPAAAARAEAAABUAAAAABAAAAAAAQEBHwALLwAAMQFMAwAAAB8AARwAAAAAAQICHwABLAoAAAAfAAAsAgAAAAABPAUAAAAfAAAhASMAAAAAAQICHwALLBEAAAAfAAwsCQAAAAcBTGF5ZXIgMTwFAAAAHwAAIQEaAAAAAAEBBB8AAS8ADT8AAE8AAFQAAAAAbAQAAAACLwALXgAAAAABAQMfAAsvAQ4/AABPAABUAAAAAGxIAAAAASQKAAAANAsAAABECQAAAFwNAAAACwFIZWxsbyB3b3JsZGwhAAAAAQAAAAAAAPA/AAAAAAAAAEAAAAAAAAAIQAAAAAAAABBA"

	data, err := base64.StdEncoding.DecodeString(fixtureBase64)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	blocks, err := ParseBlocks(data)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	var glyphs []GlyphRange
	for _, b := range blocks {
		if b.Type != BlockSceneGlyphItem {
			continue
		}
		gd, ok := b.Data.(*SceneGlyphData)
		if !ok || gd == nil {
			t.Fatalf("expected SceneGlyphData, got %#v", b.Data)
		}
		glyphs = append(glyphs, gd.Glyph)
	}

	if len(glyphs) != 1 {
		t.Fatalf("expected 1 glyph, got %d", len(glyphs))
	}

	got := glyphs[0]
	if got.Start == nil || *got.Start != 10 {
		t.Fatalf("expected start 10, got %#v", got.Start)
	}
	if got.Length != 11 {
		t.Fatalf("expected length 11, got %d", got.Length)
	}
	if got.Text != "Hello world" {
		t.Fatalf("expected text %q, got %q", "Hello world", got.Text)
	}
	if got.Color != 9 {
		t.Fatalf("expected highlight color 9, got %d", got.Color)
	}
	if len(got.Rects) != 1 {
		t.Fatalf("expected 1 rectangle, got %d", len(got.Rects))
	}
	if got.Rects[0].W != 3 || got.Rects[0].H != 4 {
		t.Fatalf("unexpected rectangle: %#v", got.Rects[0])
	}
}
