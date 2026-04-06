package rm

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// BuildPage generates a complete valid v6 .rm file matching real device format.
// Block order and versions match what xochitl 3.27 produces.
func BuildPage(lines []Line, authorUUID string) ([]byte, error) {
	w := NewWriter()
	w.WriteHeader()

	uuidBytes, err := uuidToLE(authorUUID)
	if err != nil {
		return nil, fmt.Errorf("invalid UUID %q: %w", authorUUID, err)
	}

	// block order matches real .rm files from Paper Pro fw 3.27:
	// 1. AuthorIds (ver=1)
	// 2. MigrationInfo (ver=1)
	// 3. PageInfo (ver=1)
	// 4. SceneInfo (ver=1)
	// 5. SceneTree (ver=1) — single block with root + layer
	// 6. TreeNode root (ver=2)
	// 7. TreeNode layer (ver=2)
	// 8. GroupItem (ver=1)
	// 9+ LineItems (ver=2)

	writeAuthorIds(w, uuidBytes)
	writeMigrationInfo(w)
	writePageInfo(w)

	layerID := CrdtId{1, 2}
	writeSceneInfo(w, layerID)
	writeSceneTree(w, layerID)
	writeTreeNodeRoot(w)
	writeTreeNodeLayer(w, layerID)
	writeGroupItem(w, layerID, lines)

	for i, line := range lines {
		itemID := CrdtId{1, uint64(4 + i)}
		writeLineItem(w, layerID, itemID, line)
	}

	return w.Bytes(), nil
}

// writeBlockEnvelope writes block envelope + payload
func writeBlockEnvelope(w *Writer, blockType BlockType, version uint8, payload []byte) {
	w.WriteUint32(uint32(len(payload)))
	w.WriteUint8(0)
	w.WriteUint8(1)       // min_version = 1
	w.WriteUint8(version) // cur_version
	w.WriteUint8(uint8(blockType))
	w.WriteBytes(payload)
}

// --- blocks in device order ---

func writeAuthorIds(w *Writer, uuidBytes []byte) {
	sub := NewWriter()
	sub.WriteVaruint(1) // 1 author

	sub.WriteTag(0, TagLength4)
	innerLen := uint32(1 + 16 + 2) // varuint(16) + 16 uuid bytes + uint16 author_id
	sub.WriteUint32(innerLen)
	sub.WriteVaruint(16)
	sub.WriteBytes(uuidBytes)
	sub.WriteUint16(1) // author_id = 1

	writeBlockEnvelope(w, BlockAuthorIds, 1, sub.Bytes())
}

func writeMigrationInfo(w *Writer) {
	sub := NewWriter()
	sub.WriteTag(1, TagID)
	sub.WriteCrdtId(CrdtId{1, 1})
	sub.WriteTag(2, TagByte1)
	sub.WriteByte(1) // is_device = true
	writeBlockEnvelope(w, BlockMigrationInfo, 1, sub.Bytes())
}

func writePageInfo(w *Writer) {
	sub := NewWriter()
	sub.WriteTag(1, TagByte4)
	sub.WriteUint32(1) // loads_count
	sub.WriteTag(2, TagByte4)
	sub.WriteUint32(0) // merges_count
	writeBlockEnvelope(w, BlockPageInfo, 1, sub.Bytes())
}

func writeSceneInfo(w *Writer, currentLayer CrdtId) {
	sub := NewWriter()

	// current_layer as LWW<CrdtId>
	inner := NewWriter()
	inner.WriteTag(1, TagID)
	inner.WriteCrdtId(CrdtId{1, 1}) // timestamp
	inner.WriteTag(2, TagID)
	inner.WriteCrdtId(currentLayer)

	sub.WriteTag(1, TagLength4)
	sub.WriteUint32(uint32(len(inner.Bytes())))
	sub.WriteBytes(inner.Bytes())

	writeBlockEnvelope(w, BlockSceneInfo, 1, sub.Bytes())
}

func writeSceneTree(w *Writer, layerID CrdtId) {
	sub := NewWriter()

	// root node
	sub.WriteTag(1, TagID)
	sub.WriteCrdtId(CrdtId{0, 1}) // tree_id = root

	sub.WriteTag(2, TagID)
	sub.WriteCrdtId(layerID) // node_id = layer

	sub.WriteTag(3, TagID)
	sub.WriteCrdtId(CrdtId{0, 1}) // parent_id = root

	writeBlockEnvelope(w, BlockSceneTree, 1, sub.Bytes())
}

func writeTreeNodeRoot(w *Writer) {
	sub := NewWriter()
	sub.WriteTag(1, TagID)
	sub.WriteCrdtId(CrdtId{0, 1})
	writeBlockEnvelope(w, BlockTreeNode, 2, sub.Bytes())
}

func writeTreeNodeLayer(w *Writer, layerID CrdtId) {
	sub := NewWriter()
	sub.WriteTag(1, TagID)
	sub.WriteCrdtId(layerID)

	// visible LWW = true
	visInner := NewWriter()
	visInner.WriteTag(1, TagID)
	visInner.WriteCrdtId(CrdtId{1, 1})
	visInner.WriteTag(2, TagByte1)
	visInner.WriteByte(1)

	sub.WriteTag(3, TagLength4)
	sub.WriteUint32(uint32(len(visInner.Bytes())))
	sub.WriteBytes(visInner.Bytes())

	writeBlockEnvelope(w, BlockTreeNode, 2, sub.Bytes())
}

func writeGroupItem(w *Writer, parentID CrdtId, lines []Line) {
	sub := NewWriter()

	sub.WriteTag(1, TagID)
	sub.WriteCrdtId(parentID)

	groupID := CrdtId{1, 3}
	sub.WriteTag(2, TagID)
	sub.WriteCrdtId(groupID)

	sub.WriteTag(3, TagID)
	sub.WriteCrdtId(CrdtId{}) // leftID

	sub.WriteTag(4, TagID)
	sub.WriteCrdtId(CrdtId{}) // rightID

	sub.WriteTag(5, TagByte4)
	sub.WriteUint32(0) // deleted_length

	// value sub-block: group type (0x02) + node ref
	inner := NewWriter()
	inner.WriteUint8(0x02)
	inner.WriteTag(1, TagID)
	inner.WriteCrdtId(groupID)

	sub.WriteTag(6, TagLength4)
	sub.WriteUint32(uint32(len(inner.Bytes())))
	sub.WriteBytes(inner.Bytes())

	writeBlockEnvelope(w, BlockSceneGroupItem, 1, sub.Bytes())
}

func writeLineItem(w *Writer, parentID CrdtId, itemID CrdtId, line Line) {
	sub := NewWriter()

	sub.WriteTag(1, TagID)
	sub.WriteCrdtId(parentID)

	sub.WriteTag(2, TagID)
	sub.WriteCrdtId(itemID)

	sub.WriteTag(3, TagID)
	sub.WriteCrdtId(CrdtId{}) // leftID

	sub.WriteTag(4, TagID)
	sub.WriteCrdtId(CrdtId{}) // rightID

	sub.WriteTag(5, TagByte4)
	sub.WriteUint32(0) // deleted_length

	// line value sub-block
	inner := NewWriter()
	inner.WriteUint8(0x03) // item type = line

	inner.WriteTag(1, TagByte4)
	inner.WriteUint32(uint32(line.Tool))

	inner.WriteTag(2, TagByte4)
	inner.WriteUint32(uint32(line.Color))

	inner.WriteTag(3, TagByte8)
	inner.WriteFloat64(line.ThicknessScale)

	pointData := NewWriter()
	pointData.WritePointsV2(line.Points)
	inner.WriteTag(5, TagLength4)
	inner.WriteUint32(uint32(len(pointData.Bytes())))
	inner.WriteBytes(pointData.Bytes())

	sub.WriteTag(6, TagLength4)
	sub.WriteUint32(uint32(len(inner.Bytes())))
	sub.WriteBytes(inner.Bytes())

	writeBlockEnvelope(w, BlockSceneLineItem, 2, sub.Bytes())
}

// --- UUID helpers ---

func uuidToLE(uuid string) ([]byte, error) {
	clean := strings.ReplaceAll(uuid, "-", "")
	if len(clean) != 32 {
		return nil, fmt.Errorf("UUID must be 32 hex chars, got %d", len(clean))
	}

	raw, err := hex.DecodeString(clean)
	if err != nil {
		return nil, err
	}

	le := make([]byte, 16)
	le[0], le[1], le[2], le[3] = raw[3], raw[2], raw[1], raw[0]
	le[4], le[5] = raw[5], raw[4]
	le[6], le[7] = raw[7], raw[6]
	copy(le[8:], raw[8:])

	return le, nil
}
