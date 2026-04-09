package rm

import (
	"fmt"
	"log"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
)

// Block is a parsed top-level block from a v6 .rm file
type Block struct {
	Type    BlockType
	Version uint8
	Data    any
}

// ParseBlocks reads all blocks from a v6 .rm file
func ParseBlocks(data []byte) ([]Block, error) {
	r := NewReader(data)
	if err := r.ReadHeader(); err != nil {
		return nil, err
	}

	var blocks []Block
	for r.Remaining() > 0 {
		block, err := parseBlock(r)
		if err != nil {
			return blocks, fmt.Errorf("at offset %d: %w", r.Position(), err)
		}
		blocks = append(blocks, block)
	}

	return blocks, nil
}

func parseBlock(r *Reader) (Block, error) {
	blockLen, _, curVer, blockType, err := r.ReadBlockEnvelope()
	if err != nil {
		return Block{}, err
	}

	startPos := r.Position()
	endPos := startPos + int(blockLen)
	if endPos > len(r.data) {
		endPos = len(r.data)
	}

	// sub-reader scoped to this block
	sub := &Reader{data: r.data[:endPos], pos: r.pos}
	var data any

	switch blockType {
	case BlockAuthorIds:
		data, err = parseAuthorIds(sub)
	case BlockSceneGlyphItem:
		data, err = parseSceneGlyphItem(sub)
	case BlockSceneLineItem:
		data, err = parseSceneLineItem(sub, curVer)
	default:
		// skip all other block types (scene tree, text, groups, etc.)
		// they're parsed but data isn't consumed yet
	}

	if err != nil {
		log.Printf("warning: block 0x%02X parse error at offset %d: %v", blockType, startPos, err)
		err = nil
	}

	r.pos = endPos
	return Block{Type: blockType, Version: curVer, Data: data}, nil
}

// --- AuthorIds block ---

// AuthorIdsData maps author uint8 IDs to UUID strings
type AuthorIdsData struct {
	Authors map[uint8]string
}

func parseAuthorIds(r *Reader) (*AuthorIdsData, error) {
	result := &AuthorIdsData{Authors: make(map[uint8]string)}

	count, err := r.ReadVaruint()
	if err != nil {
		return result, err
	}

	for i := uint64(0); i < count; i++ {
		_, tagType, err := r.ReadTag()
		if err != nil {
			return result, err
		}
		if tagType != TagLength4 {
			return result, fmt.Errorf("expected Length4 tag in AuthorIds, got %d", tagType)
		}

		subLen, err := r.ReadUint32()
		if err != nil {
			return result, err
		}
		subEnd := r.Position() + int(subLen)

		uuidLen, _ := r.ReadVaruint()
		uuidBytes, _ := r.ReadBytes(int(uuidLen))
		uuidStr := formatUUIDFromLE(uuidBytes)

		authorID, _ := r.ReadUint16()
		result.Authors[uint8(authorID)] = uuidStr

		if r.Position() < subEnd {
			r.Skip(subEnd - r.Position())
		}
	}

	return result, nil
}

func formatUUIDFromLE(b []byte) string {
	if len(b) != 16 {
		return fmt.Sprintf("%x", b)
	}
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[3], b[2], b[1], b[0], b[5], b[4], b[7], b[6],
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}

// --- SceneLineItem block (strokes) ---

// SceneLineData holds a parsed stroke
type SceneLineData struct {
	ParentID CrdtId
	ItemID   CrdtId
	Line     Line
}

// SceneGlyphData holds a parsed PDF/text highlight range.
type SceneGlyphData struct {
	ParentID CrdtId
	ItemID   CrdtId
	Glyph    GlyphRange
}

func parseSceneGlyphItem(r *Reader) (*SceneGlyphData, error) {
	result := &SceneGlyphData{}

	for r.Remaining() > 0 {
		index, tagType, err := r.ReadTag()
		if err != nil {
			break
		}

		switch {
		case index == 1 && tagType == TagID:
			result.ParentID, _ = r.ReadCrdtId()
		case index == 2 && tagType == TagID:
			result.ItemID, _ = r.ReadCrdtId()
		case index == 3 && tagType == TagID:
			r.ReadCrdtId() // leftID
		case index == 4 && tagType == TagID:
			r.ReadCrdtId() // rightID
		case index == 5 && tagType == TagByte4:
			r.ReadUint32() // deleted length
		case index == 6 && tagType == TagLength4:
			subLen, _ := r.ReadUint32()
			subEnd := r.Position() + int(subLen)

			itemType, _ := r.ReadUint8()
			if itemType != 0x01 {
				r.Skip(subEnd - r.Position())
				break
			}

			result.Glyph, _ = parseGlyphFields(r, subEnd)
		default:
			skipTagValue(r, tagType)
		}
	}

	return result, nil
}

func parseGlyphFields(r *Reader, endPos int) (GlyphRange, error) {
	var glyph GlyphRange
	var startValue int
	var hasStart bool

	for r.Position() < endPos {
		index, tagType, err := r.ReadTag()
		if err != nil {
			break
		}

		switch {
		case index == 2 && tagType == TagByte4:
			v, _ := r.ReadUint32()
			startValue = int(v)
			hasStart = true
		case index == 3 && tagType == TagByte4:
			v, _ := r.ReadUint32()
			glyph.Length = int(v)
		case index == 4 && tagType == TagByte4:
			v, _ := r.ReadUint32()
			glyph.Color = model.PenColor(v)
		case index == 5 && tagType == TagLength4:
			subLen, _ := r.ReadUint32()
			subEnd := r.Position() + int(subLen)
			glyph.Text, _ = r.ReadString()
			if r.Position() < subEnd {
				r.Skip(subEnd - r.Position())
			}
		case index == 6 && tagType == TagLength4:
			subLen, _ := r.ReadUint32()
			subEnd := r.Position() + int(subLen)
			count, _ := r.ReadVaruint()
			glyph.Rects = make([]Rect, 0, count)
			for i := uint64(0); i < count; i++ {
				x, _ := r.ReadFloat64()
				y, _ := r.ReadFloat64()
				w, _ := r.ReadFloat64()
				h, _ := r.ReadFloat64()
				glyph.Rects = append(glyph.Rects, Rect{X: x, Y: y, W: w, H: h})
			}
			if r.Position() < subEnd {
				r.Skip(subEnd - r.Position())
			}
		case index == 7 && tagType == TagByte4:
			glyph.ColorRGBA, _ = r.ReadUint32()
		default:
			skipTagValue(r, tagType)
		}
	}

	if hasStart {
		glyph.Start = &startValue
	}
	if glyph.Length == 0 {
		glyph.Length = len(glyph.Text)
	}

	return glyph, nil
}

func parseSceneLineItem(r *Reader, version uint8) (*SceneLineData, error) {
	result := &SceneLineData{}

	for r.Remaining() > 0 {
		index, tagType, err := r.ReadTag()
		if err != nil {
			break
		}

		switch {
		case index == 1 && tagType == TagID:
			result.ParentID, _ = r.ReadCrdtId()
		case index == 2 && tagType == TagID:
			result.ItemID, _ = r.ReadCrdtId()
		case index == 3 && tagType == TagID:
			r.ReadCrdtId() // leftID
		case index == 4 && tagType == TagID:
			r.ReadCrdtId() // rightID
		case index == 5 && tagType == TagByte4:
			r.ReadUint32() // deleted length
		case index == 6 && tagType == TagLength4:
			subLen, _ := r.ReadUint32()
			subEnd := r.Position() + int(subLen)

			itemType, _ := r.ReadUint8()
			if itemType != 0x03 {
				r.Skip(subEnd - r.Position())
				break
			}

			result.Line, _ = parseLineFields(r, subEnd)
		default:
			skipTagValue(r, tagType)
		}
	}

	return result, nil
}

func parseLineFields(r *Reader, endPos int) (Line, error) {
	var line Line

	for r.Position() < endPos {
		index, tagType, err := r.ReadTag()
		if err != nil {
			break
		}

		switch {
		case index == 1 && tagType == TagByte4:
			v, _ := r.ReadUint32()
			line.Tool = model.PenType(v)
		case index == 2 && tagType == TagByte4:
			v, _ := r.ReadUint32()
			line.Color = model.PenColor(v)
		case index == 3 && tagType == TagByte8:
			line.ThicknessScale, _ = r.ReadFloat64()
		case index == 4 && tagType == TagByte4:
			line.StartingLength, _ = r.ReadFloat32()
		case index == 5 && tagType == TagLength4:
			dataLen, _ := r.ReadUint32()
			line.Points, _ = r.ReadPointsV2(int(dataLen))
		case index == 6 && tagType == TagID:
			line.Timestamp, _ = r.ReadCrdtId()
		case index == 7 && tagType == TagID:
			line.MoveID, _ = r.ReadCrdtId()
		case index == 8 && tagType == TagByte4:
			line.ColorRGBA, _ = r.ReadUint32()
		default:
			skipTagValue(r, tagType)
		}
	}

	return line, nil
}

func skipTagValue(r *Reader, t TagType) {
	switch t {
	case TagByte1:
		r.ReadUint8()
	case TagByte4:
		r.ReadUint32()
	case TagByte8:
		r.ReadFloat64()
	case TagLength4:
		length, _ := r.ReadUint32()
		r.Skip(int(length))
	case TagID:
		r.ReadCrdtId()
	}
}
