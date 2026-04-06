package rm

import (
	"fmt"
	"log"

	"github.com/fabioroma/remarkable-cli/pkg/model"
)

// Block is a parsed top-level block from a v6 .rm file
type Block struct {
	Type    BlockType
	Version uint8
	Data    any // type depends on BlockType
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

	// save position to enforce block length
	startPos := r.Position()
	endPos := startPos + int(blockLen)

	// create a sub-reader scoped to this block's data
	if endPos > len(r.data) {
		endPos = len(r.data)
	}
	sub := &Reader{data: r.data[:endPos], pos: r.pos}

	var data any

	switch blockType {
	case BlockAuthorIds:
		data, err = parseAuthorIds(sub)
	case BlockSceneTree:
		data, err = parseSceneTree(sub)
	case BlockTreeNode:
		data, err = parseTreeNode(sub)
	case BlockSceneLineItem:
		data, err = parseSceneLineItem(sub, curVer)
	case BlockSceneGlyphItem:
		data, err = parseSceneGlyphItem(sub)
	case BlockPageInfo:
		data, err = parsePageInfo(sub)
	case BlockSceneInfo:
		data, err = parseSceneInfo(sub)
	case BlockMigrationInfo:
		data, err = parseMigrationInfo(sub)
	case BlockSceneGroupItem, BlockSceneTextItem, BlockSceneTombstone, BlockRootText:
		// skip for now
		data = nil
		err = nil
	default:
		log.Printf("warning: unknown block type 0x%02X at offset %d", blockType, startPos)
		data = nil
	}

	// ignore parse errors within blocks, just log them
	if err != nil {
		log.Printf("warning: block 0x%02X parse error at offset %d: %v", blockType, startPos, err)
		err = nil
	}

	// always advance the main reader past this block
	r.pos = endPos

	return Block{Type: blockType, Version: curVer, Data: data}, nil
}

// --- block parsers ---

// AuthorIdsData maps author uint8 IDs to UUID strings
type AuthorIdsData struct {
	Authors map[uint8]string
}

func parseAuthorIds(r *Reader) (*AuthorIdsData, error) {
	result := &AuthorIdsData{Authors: make(map[uint8]string)}

	// format: varuint count, then for each entry:
	//   tag (varuint, field 0 + Length4)
	//   uint32 subblock_len
	//   varuint uuid_len (16)
	//   [16]byte uuid (little-endian)
	//   uint16 author_id

	count, err := r.ReadVaruint()
	if err != nil {
		return result, err
	}

	for i := uint64(0); i < count; i++ {
		// read tag (expect Length4 subblock)
		_, tagType, err := r.ReadTag()
		if err != nil {
			return result, err
		}
		if tagType != TagLength4 {
			return result, fmt.Errorf("expected Length4 tag in AuthorIds, got %d", tagType)
		}

		// subblock length
		subLen, err := r.ReadUint32()
		if err != nil {
			return result, err
		}
		subEnd := r.Position() + int(subLen)

		// uuid length (varuint, always 16)
		uuidLen, err := r.ReadVaruint()
		if err != nil {
			return result, err
		}

		// uuid bytes (little-endian)
		uuidBytes, err := r.ReadBytes(int(uuidLen))
		if err != nil {
			return result, err
		}

		// convert little-endian UUID bytes to standard string
		uuidStr := formatUUIDFromLE(uuidBytes)

		// author id (uint16 LE)
		authorID, err := r.ReadUint16()
		if err != nil {
			return result, err
		}

		result.Authors[uint8(authorID)] = uuidStr

		// skip any remaining bytes in subblock
		if r.Position() < subEnd {
			r.Skip(subEnd - r.Position())
		}
	}

	return result, nil
}

// formatUUIDFromLE converts 16 little-endian UUID bytes to standard string format
func formatUUIDFromLE(b []byte) string {
	if len(b) != 16 {
		return fmt.Sprintf("%x", b)
	}

	// little-endian UUID: first 4 bytes reversed, next 2 reversed, next 2 reversed, rest as-is
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[3], b[2], b[1], b[0],
		b[5], b[4],
		b[7], b[6],
		b[8], b[9],
		b[10], b[11], b[12], b[13], b[14], b[15])
}

// SceneTreeData holds parent-child relationships
type SceneTreeData struct {
	TreeID   CrdtId
	NodeID   CrdtId
	ParentID CrdtId
}

func parseSceneTree(r *Reader) (*SceneTreeData, error) {
	result := &SceneTreeData{}

	for {
		if r.Remaining() <= 0 {
			break
		}

		index, tagType, err := r.ReadTag()
		if err != nil {
			break
		}

		switch {
		case index == 1 && tagType == TagID:
			result.TreeID, _ = r.ReadCrdtId()
		case index == 2 && tagType == TagID:
			result.NodeID, _ = r.ReadCrdtId()
		case index == 3 && tagType == TagID:
			result.ParentID, _ = r.ReadCrdtId()
		default:
			skipTagValue(r, tagType)
		}
	}

	return result, nil
}

// TreeNodeData holds node properties (label, visibility)
type TreeNodeData struct {
	NodeID  CrdtId
	Label   string
	Visible bool
}

func parseTreeNode(r *Reader) (*TreeNodeData, error) {
	result := &TreeNodeData{Visible: true}

	for {
		if r.Remaining() <= 0 {
			break
		}

		index, tagType, err := r.ReadTag()
		if err != nil {
			break
		}

		switch {
		case index == 1 && tagType == TagID:
			result.NodeID, _ = r.ReadCrdtId()
		default:
			skipTagValue(r, tagType)
		}
	}

	return result, nil
}

// SceneLineData holds a parsed stroke
type SceneLineData struct {
	ParentID CrdtId
	ItemID   CrdtId
	Line     Line
}

func parseSceneLineItem(r *Reader, version uint8) (*SceneLineData, error) {
	result := &SceneLineData{}

	// scene item envelope: parentID, itemID, leftID, rightID, deleted, value
	for {
		if r.Remaining() <= 0 {
			break
		}

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
			// leftID, skip
			r.ReadCrdtId()

		case index == 4 && tagType == TagID:
			// rightID, skip
			r.ReadCrdtId()

		case index == 5 && tagType == TagByte4:
			// deleted length
			r.ReadUint32()

		case index == 6 && tagType == TagLength4:
			// value sub-block
			subLen, _ := r.ReadUint32()
			subEnd := r.Position() + int(subLen)

			// item type (should be 0x03 for line)
			itemType, _ := r.ReadUint8()
			if itemType != 0x03 {
				r.Skip(subEnd - r.Position())
				break
			}

			// parse line fields
			result.Line, err = parseLineFields(r, subEnd)
			if err != nil {
				r.Skip(subEnd - r.Position())
			}

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
			// raw point data
			dataLen, _ := r.ReadUint32()
			points, _ := r.ReadPointsV2(int(dataLen))
			line.Points = points

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

func parseSceneGlyphItem(r *Reader) (*GlyphRange, error) {
	// skip for now, just consume the block
	return nil, nil
}

// PageInfoData holds page statistics
type PageInfoData struct {
	LoadsCount  uint32
	MergesCount uint32
}

func parsePageInfo(r *Reader) (*PageInfoData, error) {
	result := &PageInfoData{}

	for {
		if r.Remaining() <= 0 {
			break
		}

		index, tagType, err := r.ReadTag()
		if err != nil {
			break
		}

		switch {
		case index == 1 && tagType == TagByte4:
			result.LoadsCount, _ = r.ReadUint32()
		case index == 2 && tagType == TagByte4:
			result.MergesCount, _ = r.ReadUint32()
		default:
			skipTagValue(r, tagType)
		}
	}

	return result, nil
}

// SceneInfoData holds current layer and display settings
type SceneInfoData struct {
	CurrentLayer CrdtId
}

func parseSceneInfo(r *Reader) (*SceneInfoData, error) {
	result := &SceneInfoData{}

	for {
		if r.Remaining() <= 0 {
			break
		}

		index, tagType, err := r.ReadTag()
		if err != nil {
			break
		}

		switch {
		case index == 1 && tagType == TagID:
			result.CurrentLayer, _ = r.ReadCrdtId()
		default:
			skipTagValue(r, tagType)
		}
	}

	return result, nil
}

// MigrationInfoData holds migration metadata
type MigrationInfoData struct {
	MigrationID CrdtId
	IsDevice    bool
}

func parseMigrationInfo(r *Reader) (*MigrationInfoData, error) {
	result := &MigrationInfoData{}

	for {
		if r.Remaining() <= 0 {
			break
		}

		index, tagType, err := r.ReadTag()
		if err != nil {
			break
		}

		switch {
		case index == 1 && tagType == TagID:
			result.MigrationID, _ = r.ReadCrdtId()
		case index == 2 && tagType == TagByte1:
			v, _ := r.ReadUint8()
			result.IsDevice = v != 0
		default:
			skipTagValue(r, tagType)
		}
	}

	return result, nil
}

// skipTagValue skips a tagged value based on its type
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
