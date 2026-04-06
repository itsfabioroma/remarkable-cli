package rm

import (
	"testing"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
)

func TestBuildPage_RoundTrip(t *testing.T) {
	// create a simple line with 3 points
	line := Line{
		Tool:           model.PenFineliner,
		Color:          model.ColorBlack,
		ThicknessScale: 2.0,
		Points: []Point{
			{X: 100, Y: 200, Speed: 10, Width: 20, Direction: 0, Pressure: 128},
			{X: 150, Y: 250, Speed: 15, Width: 22, Direction: 1, Pressure: 130},
			{X: 200, Y: 300, Speed: 20, Width: 24, Direction: 2, Pressure: 132},
		},
	}

	// build page
	data, err := BuildPage([]Line{line}, "a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	if err != nil {
		t.Fatalf("BuildPage failed: %v", err)
	}

	// round-trip parse
	blocks, err := ParseBlocks(data)
	if err != nil {
		t.Fatalf("ParseBlocks failed: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("no blocks parsed")
	}

	// verify block types present
	counts := make(map[BlockType]int)
	for _, b := range blocks {
		counts[b.Type]++
	}
	t.Logf("block counts: %v", counts)

	if counts[BlockMigrationInfo] != 1 {
		t.Errorf("expected 1 MigrationInfo, got %d", counts[BlockMigrationInfo])
	}
	if counts[BlockAuthorIds] != 1 {
		t.Errorf("expected 1 AuthorIds, got %d", counts[BlockAuthorIds])
	}
	if counts[BlockPageInfo] != 1 {
		t.Errorf("expected 1 PageInfo, got %d", counts[BlockPageInfo])
	}
	if counts[BlockSceneLineItem] != 1 {
		t.Errorf("expected 1 SceneLineItem, got %d", counts[BlockSceneLineItem])
	}

	// verify the SceneLineItem round-trips correctly
	for _, b := range blocks {
		if b.Type != BlockSceneLineItem {
			continue
		}

		lineData, ok := b.Data.(*SceneLineData)
		if !ok || lineData == nil {
			t.Fatal("SceneLineItem data is nil or wrong type")
		}

		// check point count
		if len(lineData.Line.Points) != 3 {
			t.Errorf("expected 3 points, got %d", len(lineData.Line.Points))
		}

		// check tool
		if lineData.Line.Tool != model.PenFineliner {
			t.Errorf("expected tool Fineliner, got %d", lineData.Line.Tool)
		}

		// check color
		if lineData.Line.Color != model.ColorBlack {
			t.Errorf("expected color Black, got %d", lineData.Line.Color)
		}

		// check thickness
		if lineData.Line.ThicknessScale != 2.0 {
			t.Errorf("expected thickness 2.0, got %f", lineData.Line.ThicknessScale)
		}

		// verify point values
		pt := lineData.Line.Points[0]
		if pt.X != 100 || pt.Y != 200 {
			t.Errorf("point 0: expected (100,200), got (%.1f,%.1f)", pt.X, pt.Y)
		}

		pt2 := lineData.Line.Points[2]
		if pt2.X != 200 || pt2.Y != 300 {
			t.Errorf("point 2: expected (200,300), got (%.1f,%.1f)", pt2.X, pt2.Y)
		}
	}

	// verify author IDs round-trip
	for _, b := range blocks {
		if b.Type != BlockAuthorIds {
			continue
		}
		authors, ok := b.Data.(*AuthorIdsData)
		if !ok || authors == nil {
			t.Fatal("AuthorIds data is nil")
		}
		uuid, exists := authors.Authors[1]
		if !exists {
			t.Error("author ID 1 not found")
		} else if uuid != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
			t.Errorf("UUID mismatch: got %s", uuid)
		}
	}
}

func TestBuildPage_MultipleLines(t *testing.T) {
	lines := []Line{
		{Tool: model.PenFineliner, Color: model.ColorBlack, ThicknessScale: 1.0, Points: []Point{{X: 10, Y: 20}}},
		{Tool: model.PenMarker, Color: model.ColorGray, ThicknessScale: 3.0, Points: []Point{{X: 50, Y: 60}, {X: 70, Y: 80}}},
	}

	data, err := BuildPage(lines, "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("BuildPage failed: %v", err)
	}

	blocks, err := ParseBlocks(data)
	if err != nil {
		t.Fatalf("ParseBlocks failed: %v", err)
	}

	// count line items
	lineCount := 0
	for _, b := range blocks {
		if b.Type == BlockSceneLineItem {
			lineCount++
		}
	}
	if lineCount != 2 {
		t.Errorf("expected 2 SceneLineItems, got %d", lineCount)
	}
}

func TestUuidToLE_RoundTrip(t *testing.T) {
	uuid := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	le, err := uuidToLE(uuid)
	if err != nil {
		t.Fatalf("uuidToLE failed: %v", err)
	}

	// formatUUIDFromLE should give back the original
	got := formatUUIDFromLE(le)
	if got != uuid {
		t.Errorf("UUID round-trip failed: %s -> %s", uuid, got)
	}
}
