package rm

import (
	"bytes"
	"encoding/binary"
	"math"
)

// Writer writes the v6 .rm binary format
type Writer struct {
	buf bytes.Buffer
}

// NewWriter creates a new Writer
func NewWriter() *Writer {
	return &Writer{}
}

// Bytes returns the accumulated bytes
func (w *Writer) Bytes() []byte {
	return w.buf.Bytes()
}

// WriteHeader writes the 43-byte v6 header
func (w *Writer) WriteHeader() {
	w.buf.WriteString(V6Header)
}

// WriteByte writes a single byte
func (w *Writer) WriteByte(b byte) error {
	return w.buf.WriteByte(b)
}

// WriteUint8 writes a uint8
func (w *Writer) WriteUint8(v uint8) {
	w.buf.WriteByte(v)
}

// WriteUint16 writes a little-endian uint16
func (w *Writer) WriteUint16(v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	w.buf.Write(b[:])
}

// WriteUint32 writes a little-endian uint32
func (w *Writer) WriteUint32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	w.buf.Write(b[:])
}

// WriteFloat32 writes a little-endian float32
func (w *Writer) WriteFloat32(v float32) {
	w.WriteUint32(math.Float32bits(v))
}

// WriteFloat64 writes a little-endian float64
func (w *Writer) WriteFloat64(v float64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
	w.buf.Write(b[:])
}

// WriteVaruint writes a LEB128-encoded variable-length unsigned integer
func (w *Writer) WriteVaruint(v uint64) {
	for {
		b := byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		w.buf.WriteByte(b)
		if v == 0 {
			break
		}
	}
}

// WriteTag writes a tagged field identifier
func (w *Writer) WriteTag(index uint64, t TagType) {
	w.WriteVaruint((index << 4) | uint64(t))
}

// WriteCrdtId writes a CRDT identifier
func (w *Writer) WriteCrdtId(id CrdtId) {
	w.WriteUint8(id.Part1)
	w.WriteVaruint(id.Part2)
}

// WriteBytes writes raw bytes
func (w *Writer) WriteBytes(b []byte) {
	w.buf.Write(b)
}

// WritePointsV2 writes point data in v2 format (14 bytes per point)
func (w *Writer) WritePointsV2(points []Point) {
	for _, p := range points {
		w.WriteFloat32(p.X)
		w.WriteFloat32(p.Y)
		w.WriteUint16(p.Speed)
		w.WriteUint16(p.Width)
		w.WriteUint8(p.Direction)
		w.WriteUint8(p.Pressure)
	}
}
