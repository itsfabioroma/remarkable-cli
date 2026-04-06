package rm

import (
	"encoding/binary"
	"fmt"
)

// BuildPageFromTemplate generates a v6 .rm file by:
// 1. Copying all non-LineItem blocks from a real template (preserving exact structure)
// 2. Extracting the parentID from existing LineItems or the GroupItem
// 3. Appending new LineItem blocks with the correct parentID
func BuildPageFromTemplate(lines []Line, authorUUID string, template []byte) ([]byte, error) {
	if template == nil || len(template) < 50 {
		return nil, fmt.Errorf("a real .rm template from the device is required")
	}

	// first pass: find the parentID used by existing LineItems or GroupItem
	parentID := findStrokeParent(template)

	// find the highest existing itemID sequence to avoid collisions
	maxSeq := findMaxSequence(template)

	w := NewWriter()
	w.WriteHeader()

	// second pass: copy all blocks except LineItems
	pos := len(V6Header)
	for pos+8 < len(template) {
		blockLen := binary.LittleEndian.Uint32(template[pos : pos+4])
		payloadEnd := pos + 8 + int(blockLen)
		if payloadEnd > len(template) {
			break
		}

		bt := template[pos+7]

		// keep everything except LineItems (we'll add our own)
		if BlockType(bt) != BlockSceneLineItem {
			w.WriteBytes(template[pos:payloadEnd])
		}

		pos = payloadEnd
	}

	// append our LineItem blocks using the real parentID
	for i, line := range lines {
		seq := maxSeq + uint64(i) + 1
		itemID := CrdtId{1, seq}
		writeLineItem(w, parentID, itemID, line)
	}

	return w.Bytes(), nil
}

// findStrokeParent extracts the parentID from existing LineItem or GroupItem blocks
func findStrokeParent(data []byte) CrdtId {
	blocks, err := ParseBlocks(data)
	if err != nil {
		return CrdtId{0, 11} // fallback
	}

	// get parentID from existing LineItems
	for _, b := range blocks {
		if b.Type == BlockSceneLineItem {
			if ld, ok := b.Data.(*SceneLineData); ok && !ld.ParentID.Zero() {
				return ld.ParentID
			}
		}
	}

	// fallback: common layer ID from real devices
	return CrdtId{0, 11}
}

// findMaxSequence finds the highest CRDT sequence number in use
func findMaxSequence(data []byte) uint64 {
	blocks, _ := ParseBlocks(data)
	var max uint64

	for _, b := range blocks {
		if b.Type == BlockSceneLineItem {
			if ld, ok := b.Data.(*SceneLineData); ok {
				if ld.ItemID.Part2 > max {
					max = ld.ItemID.Part2
				}
			}
		}
	}

	if max == 0 {
		max = 100 // safe starting point if no existing items
	}
	return max
}

// writeLineItem writes a LineItem block matching real device format (min=2, cur=2)
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

	// points
	pointData := NewWriter()
	pointData.WritePointsV2(line.Points)
	inner.WriteTag(5, TagLength4)
	inner.WriteUint32(uint32(len(pointData.Bytes())))
	inner.WriteBytes(pointData.Bytes())

	sub.WriteTag(6, TagLength4)
	sub.WriteUint32(uint32(len(inner.Bytes())))
	sub.WriteBytes(inner.Bytes())

	// min=2, cur=2 to match real device files
	w.WriteUint32(uint32(len(sub.Bytes())))
	w.WriteByte(0)
	w.WriteByte(2) // min_version
	w.WriteByte(2) // cur_version
	w.WriteByte(uint8(BlockSceneLineItem))
	w.WriteBytes(sub.Bytes())
}

// ParseBlobToTemplate strips LineItem blocks from an existing .rm file
func ParseBlobToTemplate(data []byte) ([]byte, error) {
	if len(data) < len(V6Header) {
		return nil, fmt.Errorf("data too short")
	}
	if string(data[:len(V6Header)]) != V6Header {
		return nil, fmt.Errorf("not a v6 .rm file")
	}

	w := NewWriter()
	w.WriteHeader()

	pos := len(V6Header)
	for pos+8 < len(data) {
		blockLen := binary.LittleEndian.Uint32(data[pos : pos+4])
		payloadEnd := pos + 8 + int(blockLen)
		if payloadEnd > len(data) {
			break
		}

		bt := data[pos+7]
		if BlockType(bt) != BlockSceneLineItem {
			w.WriteBytes(data[pos:payloadEnd])
		}

		pos = payloadEnd
	}

	return w.Bytes(), nil
}
