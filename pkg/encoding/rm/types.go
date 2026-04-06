package rm

import "github.com/fabioroma/remarkable-cli/pkg/model"

// v6 file header
const V6Header = "reMarkable .lines file, version=6          "

// CrdtId represents a CRDT identifier (author + sequence)
type CrdtId struct {
	Part1 uint8  // author ID
	Part2 uint64 // sequence counter
}

// Zero returns true if this is a zero/null ID
func (id CrdtId) Zero() bool {
	return id.Part1 == 0 && id.Part2 == 0
}

// LwwValue is a Last-Write-Wins register
type LwwValue[T any] struct {
	Timestamp CrdtId
	Value     T
}

// TagType identifies the wire type of a tagged field
type TagType uint8

const (
	TagByte1   TagType = 0x01 // bool or uint8
	TagByte4   TagType = 0x04 // uint32 or float32
	TagByte8   TagType = 0x08 // float64
	TagLength4 TagType = 0x0C // sub-block with uint32 length prefix
	TagID      TagType = 0x0F // CrdtId
)

// BlockType identifies the type of a top-level block
type BlockType uint8

const (
	BlockMigrationInfo    BlockType = 0x00
	BlockSceneTree        BlockType = 0x01
	BlockTreeNode         BlockType = 0x02
	BlockSceneGlyphItem   BlockType = 0x03
	BlockSceneGroupItem   BlockType = 0x04
	BlockSceneLineItem    BlockType = 0x05
	BlockSceneTextItem    BlockType = 0x06
	BlockRootText         BlockType = 0x07
	BlockSceneTombstone   BlockType = 0x08
	BlockAuthorIds        BlockType = 0x09
	BlockPageInfo         BlockType = 0x0A
	BlockSceneInfo        BlockType = 0x0D
)

// Point represents a single point in a stroke
type Point struct {
	X         float32
	Y         float32
	Speed     uint16
	Width     uint16
	Direction uint8
	Pressure  uint8
}

// Line represents a complete stroke
type Line struct {
	Tool           model.PenType
	Color          model.PenColor
	ThicknessScale float64
	StartingLength float32
	Points         []Point
	Timestamp      CrdtId
	MoveID         CrdtId   // optional, for moved strokes
	ColorRGBA      uint32   // optional, packed BGRA
}

// GlyphRange represents a PDF highlight annotation
type GlyphRange struct {
	Start     int
	Length    int
	Color     model.PenColor
	Text      string
	Rects     []Rect
	ColorRGBA uint32 // optional
}

// Rect is a bounding rectangle
type Rect struct {
	X, Y, W, H float64
}

// TextItem references text content in a RootTextBlock
type TextItem struct {
	BlockID CrdtId
	PosX    float64
	PosY    float64
	Width   float32
}

// ParagraphStyle identifies text formatting
type ParagraphStyle uint8

const (
	StyleBasic          ParagraphStyle = 0
	StylePlain          ParagraphStyle = 1
	StyleHeading        ParagraphStyle = 2
	StyleBold           ParagraphStyle = 3
	StyleBullet         ParagraphStyle = 4
	StyleBullet2        ParagraphStyle = 5
	StyleCheckbox       ParagraphStyle = 6
	StyleCheckboxDone   ParagraphStyle = 7
)

// SceneNode represents a node in the scene tree (layer/group)
type SceneNode struct {
	NodeID   CrdtId
	ParentID CrdtId
	Label    string
	Visible  bool
	Children []CrdtId
}

// Page holds all parsed content for a single .rm page
type Page struct {
	Nodes      []SceneNode
	Lines      []Line
	Glyphs     []GlyphRange
	TextItems  []TextItem
	AuthorMap  map[uint8]string // author ID -> UUID
}
