package rm

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// BuildPage generates a complete valid v6 .rm file from lines and an author UUID.
func BuildPage(lines []Line, authorUUID string) ([]byte, error) {
	w := NewWriter()
	w.WriteHeader()

	// 1. MigrationInfo (0x00): migration_id={1,1}, is_device=true
	writeMigrationInfo(w)

	// 2. AuthorIds (0x09): author_id=1, UUID in LE bytes
	uuidBytes, err := uuidToLE(authorUUID)
	if err != nil {
		return nil, fmt.Errorf("invalid UUID %q: %w", authorUUID, err)
	}
	writeAuthorIds(w, uuidBytes)

	// 3. PageInfo (0x0A): loads=1, merges=0
	writePageInfo(w)

	// 4. SceneTree root node {0,1}
	writeSceneTreeNode(w, CrdtId{0, 1}, CrdtId{})

	// 5. TreeNode for root
	writeTreeNode(w, CrdtId{0, 1}, "", false)

	// 6. SceneTree layer node {1,2}, parent=root
	writeSceneTreeNode(w, CrdtId{1, 2}, CrdtId{0, 1})

	// 7. TreeNode for layer, visible=true
	writeTreeNode(w, CrdtId{1, 2}, "", true)

	// 8. SceneGroupItem referencing strokes
	writeSceneGroupItem(w, CrdtId{1, 2}, CrdtId{1, 3})

	// 9. SceneLineItem for each line
	for i, line := range lines {
		itemID := CrdtId{1, uint64(4 + i)}
		writeSceneLineItem(w, CrdtId{1, 3}, itemID, line)
	}

	// 10. SceneInfo (0x0D): current layer
	writeSceneInfo(w, CrdtId{1, 2})

	return w.Bytes(), nil
}

// writeBlockEnvelope writes envelope + payload to main writer
func writeBlockEnvelope(w *Writer, blockType BlockType, version uint8, payload []byte) {
	w.WriteUint32(uint32(len(payload)))
	w.WriteUint8(0)        // unknown byte
	w.WriteUint8(1)        // min version
	w.WriteUint8(version)  // current version
	w.WriteUint8(uint8(blockType))
	w.WriteBytes(payload)
}

// --- block builders ---

func writeMigrationInfo(w *Writer) {
	sub := NewWriter()
	sub.WriteTag(1, TagID)
	sub.WriteCrdtId(CrdtId{1, 1})
	sub.WriteTag(2, TagByte1)
	sub.WriteByte(1) // is_device = true
	writeBlockEnvelope(w, BlockMigrationInfo, 0, sub.Bytes())
}

func writeAuthorIds(w *Writer, uuidBytes []byte) {
	sub := NewWriter()

	// count of authors
	sub.WriteVaruint(1)

	// tag(0, TagLength4) for the author entry
	sub.WriteTag(0, TagLength4)

	// sub-block: varuint(16) + 16 bytes UUID + uint16 author_id
	innerLen := uint32(1 + 16 + 2) // varuint(16)=1byte + 16 uuid + 2 author_id
	sub.WriteUint32(innerLen)
	sub.WriteVaruint(16)
	sub.WriteBytes(uuidBytes)
	sub.WriteUint16(1) // author_id = 1

	writeBlockEnvelope(w, BlockAuthorIds, 0, sub.Bytes())
}

func writePageInfo(w *Writer) {
	sub := NewWriter()
	sub.WriteTag(1, TagByte4)
	sub.WriteUint32(1) // loads_count = 1
	sub.WriteTag(2, TagByte4)
	sub.WriteUint32(0) // merges_count = 0
	writeBlockEnvelope(w, BlockPageInfo, 0, sub.Bytes())
}

func writeSceneTreeNode(w *Writer, nodeID CrdtId, parentID CrdtId) {
	sub := NewWriter()

	// tag(1, TagID) node_id
	sub.WriteTag(1, TagID)
	sub.WriteCrdtId(nodeID)

	// tag(2, TagByte1) is_update=true
	sub.WriteTag(2, TagByte1)
	sub.WriteByte(1)

	// tag(3, TagLength4) sub-block with parent info
	inner := NewWriter()

	// parent_id
	inner.WriteTag(1, TagID)
	inner.WriteCrdtId(parentID)

	sub.WriteTag(3, TagLength4)
	sub.WriteUint32(uint32(len(inner.Bytes())))
	sub.WriteBytes(inner.Bytes())

	writeBlockEnvelope(w, BlockSceneTree, 1, sub.Bytes())
}

func writeTreeNode(w *Writer, nodeID CrdtId, label string, visible bool) {
	sub := NewWriter()

	// tag(1, TagID) node_id
	sub.WriteTag(1, TagID)
	sub.WriteCrdtId(nodeID)

	// tag(2, TagLength4) label LWW
	labelInner := NewWriter()
	labelInner.WriteTag(1, TagID)
	labelInner.WriteCrdtId(CrdtId{1, 1}) // timestamp
	labelInner.WriteTag(2, TagLength4)
	labelBytes := []byte(label)
	labelInner.WriteUint32(uint32(len(labelBytes)))
	labelInner.WriteBytes(labelBytes)

	sub.WriteTag(2, TagLength4)
	sub.WriteUint32(uint32(len(labelInner.Bytes())))
	sub.WriteBytes(labelInner.Bytes())

	// tag(3, TagLength4) visible LWW
	visInner := NewWriter()
	visInner.WriteTag(1, TagID)
	visInner.WriteCrdtId(CrdtId{1, 1}) // timestamp
	visInner.WriteTag(2, TagByte1)
	if visible {
		visInner.WriteByte(1)
	} else {
		visInner.WriteByte(0)
	}

	sub.WriteTag(3, TagLength4)
	sub.WriteUint32(uint32(len(visInner.Bytes())))
	sub.WriteBytes(visInner.Bytes())

	writeBlockEnvelope(w, BlockTreeNode, 1, sub.Bytes())
}

func writeSceneGroupItem(w *Writer, parentID CrdtId, groupID CrdtId) {
	sub := NewWriter()

	// tag(1, TagID) parentID
	sub.WriteTag(1, TagID)
	sub.WriteCrdtId(parentID)

	// tag(2, TagID) itemID
	sub.WriteTag(2, TagID)
	sub.WriteCrdtId(groupID)

	// tag(3, TagID) leftID (zero)
	sub.WriteTag(3, TagID)
	sub.WriteCrdtId(CrdtId{})

	// tag(4, TagID) rightID (zero)
	sub.WriteTag(4, TagID)
	sub.WriteCrdtId(CrdtId{})

	// tag(5, TagByte4) deleted_length = 0
	sub.WriteTag(5, TagByte4)
	sub.WriteUint32(0)

	// tag(6, TagLength4) sub-block: group type (0x02) + node_id ref
	inner := NewWriter()
	inner.WriteUint8(0x02) // item type = group
	inner.WriteTag(1, TagID)
	inner.WriteCrdtId(groupID)

	sub.WriteTag(6, TagLength4)
	sub.WriteUint32(uint32(len(inner.Bytes())))
	sub.WriteBytes(inner.Bytes())

	writeBlockEnvelope(w, BlockSceneGroupItem, 0, sub.Bytes())
}

func writeSceneLineItem(w *Writer, parentID CrdtId, itemID CrdtId, line Line) {
	sub := NewWriter()

	// tag(1, TagID) parentID
	sub.WriteTag(1, TagID)
	sub.WriteCrdtId(parentID)

	// tag(2, TagID) itemID
	sub.WriteTag(2, TagID)
	sub.WriteCrdtId(itemID)

	// tag(3, TagID) leftID (zero)
	sub.WriteTag(3, TagID)
	sub.WriteCrdtId(CrdtId{})

	// tag(4, TagID) rightID (zero)
	sub.WriteTag(4, TagID)
	sub.WriteCrdtId(CrdtId{})

	// tag(5, TagByte4) deleted_length = 0
	sub.WriteTag(5, TagByte4)
	sub.WriteUint32(0)

	// tag(6, TagLength4) sub-block with line data
	inner := NewWriter()
	inner.WriteUint8(0x03) // item type = line

	// tool
	inner.WriteTag(1, TagByte4)
	inner.WriteUint32(uint32(line.Tool))

	// color
	inner.WriteTag(2, TagByte4)
	inner.WriteUint32(uint32(line.Color))

	// thickness
	inner.WriteTag(3, TagByte8)
	inner.WriteFloat64(line.ThicknessScale)

	// points
	pointData := NewWriter()
	pointData.WritePointsV2(line.Points)
	inner.WriteTag(5, TagLength4)
	inner.WriteUint32(uint32(len(pointData.Bytes())))
	inner.WriteBytes(pointData.Bytes())

	sub.WriteTag(6, TagLength4)
	sub.WriteUint32(uint32(len(inner.Bytes())))
	sub.WriteBytes(inner.Bytes())

	writeBlockEnvelope(w, BlockSceneLineItem, 1, sub.Bytes())
}

func writeSceneInfo(w *Writer, currentLayer CrdtId) {
	sub := NewWriter()

	// tag(1, TagLength4) current_layer LWW
	inner := NewWriter()
	inner.WriteTag(1, TagID)
	inner.WriteCrdtId(CrdtId{1, 1}) // timestamp
	inner.WriteTag(2, TagID)
	inner.WriteCrdtId(currentLayer)

	sub.WriteTag(1, TagLength4)
	sub.WriteUint32(uint32(len(inner.Bytes())))
	sub.WriteBytes(inner.Bytes())

	writeBlockEnvelope(w, BlockSceneInfo, 0, sub.Bytes())
}

// --- UUID helpers ---

// uuidToLE parses a UUID string and converts to reMarkable LE byte format.
// First 3 groups have byte order swapped.
func uuidToLE(uuid string) ([]byte, error) {
	clean := strings.ReplaceAll(uuid, "-", "")
	if len(clean) != 32 {
		return nil, fmt.Errorf("UUID must be 32 hex chars, got %d", len(clean))
	}

	raw, err := hex.DecodeString(clean)
	if err != nil {
		return nil, err
	}

	// swap first 3 groups: 4 bytes, 2 bytes, 2 bytes
	le := make([]byte, 16)

	// group 1: bytes 0-3 reversed
	le[0], le[1], le[2], le[3] = raw[3], raw[2], raw[1], raw[0]

	// group 2: bytes 4-5 reversed
	le[4], le[5] = raw[5], raw[4]

	// group 3: bytes 6-7 reversed
	le[6], le[7] = raw[7], raw[6]

	// groups 4-5: bytes 8-15 unchanged
	copy(le[8:], raw[8:])

	return le, nil
}
