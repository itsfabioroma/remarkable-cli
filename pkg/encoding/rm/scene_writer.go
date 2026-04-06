package rm

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

// BuildPage generates a valid v6 .rm file by cloning the envelope from a real
// device file (template) and injecting the provided strokes.
// If template is nil, uses a minimal built-in template.
func BuildPage(lines []Line, authorUUID string) ([]byte, error) {
	return BuildPageFromTemplate(lines, authorUUID, nil)
}

// BuildPageFromTemplate generates a v6 .rm file using a real .rm as template
// for the non-stroke blocks. Replaces all LineItem blocks with the provided lines.
func BuildPageFromTemplate(lines []Line, authorUUID string, template []byte) ([]byte, error) {
	if template == nil {
		template = minimalTemplate(authorUUID)
	}

	// parse template blocks
	r := NewReader(template)
	if err := r.ReadHeader(); err != nil {
		return nil, fmt.Errorf("invalid template: %w", err)
	}

	w := NewWriter()
	w.WriteHeader()

	// copy all non-LineItem blocks from template, then append our lines
	for r.Remaining() > 0 {
		blockStart := r.Position()

		blockLen, err := r.ReadUint32()
		if err != nil {
			break
		}

		// read envelope header
		unknown, _ := r.ReadUint8()
		minVer, _ := r.ReadUint8()
		curVer, _ := r.ReadUint8()
		bt, _ := r.ReadUint8()

		// read payload
		payload, _ := r.ReadBytes(int(blockLen))

		_ = blockStart

		// skip LineItem blocks from template (we'll add our own)
		if BlockType(bt) == BlockSceneLineItem {
			continue
		}

		// copy this block as-is
		w.WriteUint32(blockLen)
		w.WriteUint8(unknown)
		w.WriteUint8(minVer)
		w.WriteUint8(curVer)
		w.WriteUint8(bt)
		w.WriteBytes(payload)
	}

	// find the parent ID for line items from the GroupItem block
	parentID := CrdtId{1, 2} // default layer ID

	// append our LineItem blocks
	for i, line := range lines {
		itemID := CrdtId{1, uint64(100 + i)} // high sequence to avoid collisions
		writeLineItem(w, parentID, itemID, line)
	}

	return w.Bytes(), nil
}

// minimalTemplate builds a template by reading block structures from
// a known-good format, matching what real devices produce.
func minimalTemplate(authorUUID string) []byte {
	uuidBytes, err := uuidToLE(authorUUID)
	if err != nil {
		uuidBytes = make([]byte, 16) // fallback: zeros
	}

	w := NewWriter()
	w.WriteHeader()

	// AuthorIds (ver=1)
	writeAuthorIds(w, uuidBytes)

	// MigrationInfo (ver=1) — 3 fields to match device
	{
		sub := NewWriter()
		sub.WriteTag(1, TagID)
		sub.WriteCrdtId(CrdtId{1, 1})
		sub.WriteTag(2, TagByte1)
		sub.WriteByte(1)
		sub.WriteTag(3, TagByte1)
		sub.WriteByte(0)
		writeBlockEnvelope(w, BlockMigrationInfo, 1, sub.Bytes())
	}

	// PageInfo (min=0, ver=1) — 5 uint32 fields to match device
	{
		sub := NewWriter()
		sub.WriteTag(1, TagByte4)
		sub.WriteUint32(1) // loads_count
		sub.WriteTag(2, TagByte4)
		sub.WriteUint32(0) // merges_count
		sub.WriteTag(3, TagByte4)
		sub.WriteUint32(0)
		sub.WriteTag(4, TagByte4)
		sub.WriteUint32(0)
		sub.WriteTag(5, TagByte4)
		sub.WriteUint32(0)
		w.WriteUint32(uint32(len(sub.Bytes())))
		w.WriteUint8(0)
		w.WriteUint8(0) // min=0 (matches real file)
		w.WriteUint8(1) // cur=1
		w.WriteUint8(uint8(BlockPageInfo))
		w.WriteBytes(sub.Bytes())
	}

	// SceneInfo (min=0, ver=1) — current layer + document/background visible + paper size
	{
		sub := NewWriter()

		// tag(1) current_layer LWW
		inner1 := NewWriter()
		inner1.WriteTag(1, TagID)
		inner1.WriteCrdtId(CrdtId{0, 0}) // timestamp
		inner1.WriteTag(2, TagID)
		inner1.WriteCrdtId(CrdtId{0, 11}) // layer ref
		sub.WriteTag(1, TagLength4)
		sub.WriteUint32(uint32(len(inner1.Bytes())))
		sub.WriteBytes(inner1.Bytes())

		// tag(2) unknown LWW
		inner2 := NewWriter()
		inner2.WriteTag(1, TagID)
		inner2.WriteCrdtId(CrdtId{0, 0})
		inner2.WriteTag(2, TagByte1)
		inner2.WriteByte(1)
		sub.WriteTag(2, TagLength4)
		sub.WriteUint32(uint32(len(inner2.Bytes())))
		sub.WriteBytes(inner2.Bytes())

		// tag(3) unknown LWW
		inner3 := NewWriter()
		inner3.WriteTag(1, TagID)
		inner3.WriteCrdtId(CrdtId{0, 0})
		inner3.WriteTag(2, TagByte1)
		inner3.WriteByte(1)
		sub.WriteTag(3, TagLength4)
		sub.WriteUint32(uint32(len(inner3.Bytes())))
		sub.WriteBytes(inner3.Bytes())

		// tag(5) paper size / zoom
		inner5 := NewWriter()
		inner5.WriteTag(3, TagByte4)
		inner5.WriteUint32(0x03) // paper format?
		inner5.WriteBytes([]byte{0x00, 0x05, 0x00, 0x00, 0x00}) // padding
		sub.WriteTag(5, TagLength4)
		sub.WriteUint32(uint32(len(inner5.Bytes())))
		sub.WriteBytes(inner5.Bytes())

		// tag(6) unknown
		inner6 := NewWriter()
		inner6.WriteTag(1, TagID)
		inner6.WriteCrdtId(CrdtId{0, 0})
		inner6.WriteTag(2, TagByte4)
		inner6.WriteUint32(0x20) // 32
		sub.WriteTag(6, TagLength4)
		sub.WriteUint32(uint32(len(inner6.Bytes())))
		sub.WriteBytes(inner6.Bytes())

		w.WriteUint32(uint32(len(sub.Bytes())))
		w.WriteUint8(0)
		w.WriteUint8(0) // min=0
		w.WriteUint8(1)
		w.WriteUint8(uint8(BlockSceneInfo))
		w.WriteBytes(sub.Bytes())
	}

	// SceneTree (ver=1)
	{
		sub := NewWriter()
		sub.WriteTag(1, TagID)
		sub.WriteCrdtId(CrdtId{0, 1})
		sub.WriteTag(2, TagID)
		sub.WriteCrdtId(CrdtId{0, 11})
		sub.WriteTag(3, TagByte1)
		sub.WriteByte(1)
		sub.WriteTag(4, TagLength4)
		sub.WriteUint32(3)
		sub.WriteTag(1, TagID)
		sub.WriteCrdtId(CrdtId{0, 1})
		writeBlockEnvelope(w, BlockSceneTree, 1, sub.Bytes())
	}

	// TreeNode root (ver=2)
	{
		sub := NewWriter()
		sub.WriteTag(1, TagID)
		sub.WriteCrdtId(CrdtId{0, 1})
		writeBlockEnvelope(w, BlockTreeNode, 2, sub.Bytes())
	}

	// TreeNode layer (ver=2)
	{
		sub := NewWriter()
		sub.WriteTag(1, TagID)
		sub.WriteCrdtId(CrdtId{0, 11})

		// label LWW
		labelInner := NewWriter()
		labelInner.WriteTag(1, TagID)
		labelInner.WriteCrdtId(CrdtId{0, 12})
		labelInner.WriteTag(2, TagLength4)
		label := []byte("Layer 1")
		labelInner.WriteUint32(uint32(len(label)))
		labelInner.WriteBytes(label)
		sub.WriteTag(2, TagLength4)
		sub.WriteUint32(uint32(len(labelInner.Bytes())))
		sub.WriteBytes(labelInner.Bytes())

		// visible LWW
		visInner := NewWriter()
		visInner.WriteTag(1, TagID)
		visInner.WriteCrdtId(CrdtId{0, 0})
		visInner.WriteTag(2, TagByte1)
		visInner.WriteByte(1)
		sub.WriteTag(3, TagLength4)
		sub.WriteUint32(uint32(len(visInner.Bytes())))
		sub.WriteBytes(visInner.Bytes())

		writeBlockEnvelope(w, BlockTreeNode, 2, sub.Bytes())
	}

	// GroupItem (ver=1) — empty group
	{
		sub := NewWriter()
		sub.WriteTag(1, TagID)
		sub.WriteCrdtId(CrdtId{0, 1})
		sub.WriteTag(2, TagID)
		sub.WriteCrdtId(CrdtId{0, 13})
		sub.WriteTag(3, TagID)
		sub.WriteCrdtId(CrdtId{})
		sub.WriteTag(4, TagID)
		sub.WriteCrdtId(CrdtId{})
		sub.WriteTag(5, TagByte4)
		sub.WriteUint32(0)
		inner := NewWriter()
		inner.WriteUint8(0x02)
		inner.WriteTag(2, TagID)
		inner.WriteCrdtId(CrdtId{0, 11})
		sub.WriteTag(6, TagLength4)
		sub.WriteUint32(uint32(len(inner.Bytes())))
		sub.WriteBytes(inner.Bytes())
		writeBlockEnvelope(w, BlockSceneGroupItem, 1, sub.Bytes())
	}

	return w.Bytes()
}

func writeBlockEnvelope(w *Writer, blockType BlockType, version uint8, payload []byte) {
	w.WriteUint32(uint32(len(payload)))
	w.WriteUint8(0)
	w.WriteUint8(1)
	w.WriteUint8(version)
	w.WriteUint8(uint8(blockType))
	w.WriteBytes(payload)
}

func writeAuthorIds(w *Writer, uuidBytes []byte) {
	sub := NewWriter()
	sub.WriteVaruint(1)
	sub.WriteTag(0, TagLength4)
	sub.WriteUint32(19) // 1 + 16 + 2
	sub.WriteVaruint(16)
	sub.WriteBytes(uuidBytes)
	sub.WriteUint16(1)
	writeBlockEnvelope(w, BlockAuthorIds, 1, sub.Bytes())
}

func writeLineItem(w *Writer, parentID CrdtId, itemID CrdtId, line Line) {
	sub := NewWriter()

	sub.WriteTag(1, TagID)
	sub.WriteCrdtId(parentID)
	sub.WriteTag(2, TagID)
	sub.WriteCrdtId(itemID)
	sub.WriteTag(3, TagID)
	sub.WriteCrdtId(CrdtId{})
	sub.WriteTag(4, TagID)
	sub.WriteCrdtId(CrdtId{})
	sub.WriteTag(5, TagByte4)
	sub.WriteUint32(0)

	inner := NewWriter()
	inner.WriteUint8(0x03)
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

	// min=2, cur=2 to match real files
	w.WriteUint32(uint32(len(sub.Bytes())))
	w.WriteUint8(0)
	w.WriteUint8(2) // min_version = 2
	w.WriteUint8(2) // cur_version = 2
	w.WriteUint8(uint8(BlockSceneLineItem))
	w.WriteBytes(sub.Bytes())
}

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

// ParseBlobToTemplate extracts a template from an existing .rm file on the device.
// This is the most reliable way to create compatible pages — use a real page as template.
func ParseBlobToTemplate(data []byte) ([]byte, error) {
	// just validate it's a v6 file
	if len(data) < len(V6Header) {
		return nil, fmt.Errorf("data too short")
	}
	header := string(data[:len(V6Header)])
	if header != V6Header {
		return nil, fmt.Errorf("not a v6 .rm file")
	}

	// strip all LineItem blocks
	r := &Reader{data: data, pos: len(V6Header)}
	w := NewWriter()
	w.WriteHeader()

	for r.Remaining() > 0 {
		if r.pos+8 > len(r.data) {
			break
		}
		blockLen := binary.LittleEndian.Uint32(r.data[r.pos : r.pos+4])
		envelope := r.data[r.pos : r.pos+8]
		bt := envelope[7]

		payloadEnd := r.pos + 8 + int(blockLen)
		if payloadEnd > len(r.data) {
			break
		}

		if BlockType(bt) != BlockSceneLineItem {
			w.WriteBytes(r.data[r.pos:payloadEnd])
		}

		r.pos = payloadEnd
	}

	return w.Bytes(), nil
}
