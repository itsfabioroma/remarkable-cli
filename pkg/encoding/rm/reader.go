package rm

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Reader reads the v6 .rm binary format
type Reader struct {
	data []byte
	pos  int
}

// NewReader creates a reader from raw bytes
func NewReader(data []byte) *Reader {
	return &Reader{data: data}
}

// NewReaderFromIO reads all bytes from an io.Reader
func NewReaderFromIO(r io.Reader) (*Reader, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return NewReader(data), nil
}

// ReadHeader validates the v6 file header
func (r *Reader) ReadHeader() error {
	if len(r.data) < len(V6Header) {
		return fmt.Errorf("file too short for header")
	}

	header := string(r.data[:len(V6Header)])
	if header != V6Header {
		// check for older versions
		if len(r.data) > 33 {
			prefix := string(r.data[:33])
			if prefix == "reMarkable .lines file, version=" {
				version := r.data[32]
				return fmt.Errorf("unsupported .rm version %d (only v6 is supported)", version)
			}
		}
		return fmt.Errorf("not a reMarkable .rm file")
	}

	r.pos = len(V6Header)
	return nil
}

// Remaining returns bytes left to read
func (r *Reader) Remaining() int {
	return len(r.data) - r.pos
}

// ReadByte reads a single byte
func (r *Reader) ReadByte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, io.ErrUnexpectedEOF
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

// ReadUint8 reads a uint8
func (r *Reader) ReadUint8() (uint8, error) {
	return r.ReadByte()
}

// ReadUint16 reads a little-endian uint16
func (r *Reader) ReadUint16() (uint16, error) {
	if r.pos+2 > len(r.data) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.LittleEndian.Uint16(r.data[r.pos:])
	r.pos += 2
	return v, nil
}

// ReadUint32 reads a little-endian uint32
func (r *Reader) ReadUint32() (uint32, error) {
	if r.pos+4 > len(r.data) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.LittleEndian.Uint32(r.data[r.pos:])
	r.pos += 4
	return v, nil
}

// ReadFloat32 reads a little-endian float32
func (r *Reader) ReadFloat32() (float32, error) {
	bits, err := r.ReadUint32()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(bits), nil
}

// ReadFloat64 reads a little-endian float64
func (r *Reader) ReadFloat64() (float64, error) {
	if r.pos+8 > len(r.data) {
		return 0, io.ErrUnexpectedEOF
	}
	bits := binary.LittleEndian.Uint64(r.data[r.pos:])
	r.pos += 8
	return math.Float64frombits(bits), nil
}

// ReadVaruint reads a LEB128-encoded variable-length unsigned integer
func (r *Reader) ReadVaruint() (uint64, error) {
	var result uint64
	var shift uint

	for {
		if r.pos >= len(r.data) {
			return 0, io.ErrUnexpectedEOF
		}

		b := r.data[r.pos]
		r.pos++

		result |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			return result, nil
		}

		shift += 7
		if shift >= 64 {
			return 0, fmt.Errorf("varuint overflow")
		}
	}
}

// ReadTag reads a tagged field identifier, returns (field index, tag type)
func (r *Reader) ReadTag() (uint64, TagType, error) {
	v, err := r.ReadVaruint()
	if err != nil {
		return 0, 0, err
	}

	index := v >> 4
	tagType := TagType(v & 0x0F)
	return index, tagType, nil
}

// ReadCrdtId reads a CRDT identifier
func (r *Reader) ReadCrdtId() (CrdtId, error) {
	part1, err := r.ReadUint8()
	if err != nil {
		return CrdtId{}, err
	}

	part2, err := r.ReadVaruint()
	if err != nil {
		return CrdtId{}, err
	}

	return CrdtId{Part1: part1, Part2: part2}, nil
}

// ReadBytes reads n raw bytes
func (r *Reader) ReadBytes(n int) ([]byte, error) {
	if r.pos+n > len(r.data) {
		return nil, io.ErrUnexpectedEOF
	}
	result := make([]byte, n)
	copy(result, r.data[r.pos:r.pos+n])
	r.pos += n
	return result, nil
}

// ReadString reads a length-prefixed string
func (r *Reader) ReadString() (string, error) {
	length, err := r.ReadVaruint()
	if err != nil {
		return "", err
	}

	// is_ascii flag
	_, err = r.ReadUint8()
	if err != nil {
		return "", err
	}

	data, err := r.ReadBytes(int(length))
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// Skip advances the position by n bytes
func (r *Reader) Skip(n int) error {
	if r.pos+n > len(r.data) {
		return io.ErrUnexpectedEOF
	}
	r.pos += n
	return nil
}

// Position returns the current read position
func (r *Reader) Position() int {
	return r.pos
}

// ReadBlockEnvelope reads the outer block envelope (length, header, type)
func (r *Reader) ReadBlockEnvelope() (blockLen uint32, minVer, curVer uint8, blockType BlockType, err error) {
	blockLen, err = r.ReadUint32()
	if err != nil {
		return
	}

	// unknown byte (always 0)
	_, err = r.ReadUint8()
	if err != nil {
		return
	}

	minVer, err = r.ReadUint8()
	if err != nil {
		return
	}

	curVer, err = r.ReadUint8()
	if err != nil {
		return
	}

	bt, err := r.ReadUint8()
	if err != nil {
		return
	}
	blockType = BlockType(bt)

	return
}

// ReadPointsV2 reads point data in v2 format (14 bytes per point)
func (r *Reader) ReadPointsV2(dataLen int) ([]Point, error) {
	pointSize := 14 // float32 x + float32 y + uint16 speed + uint16 width + uint8 dir + uint8 pressure
	numPoints := dataLen / pointSize

	points := make([]Point, 0, numPoints)
	for i := 0; i < numPoints; i++ {
		x, err := r.ReadFloat32()
		if err != nil {
			return points, err
		}

		y, err := r.ReadFloat32()
		if err != nil {
			return points, err
		}

		speed, err := r.ReadUint16()
		if err != nil {
			return points, err
		}

		width, err := r.ReadUint16()
		if err != nil {
			return points, err
		}

		dir, err := r.ReadUint8()
		if err != nil {
			return points, err
		}

		pressure, err := r.ReadUint8()
		if err != nil {
			return points, err
		}

		points = append(points, Point{
			X: x, Y: y,
			Speed: speed, Width: width,
			Direction: dir, Pressure: pressure,
		})
	}

	return points, nil
}
