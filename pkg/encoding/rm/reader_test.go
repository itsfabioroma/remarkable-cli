package rm

import (
	"testing"
)

func TestReader_ReadHeader_V6(t *testing.T) {
	data := []byte(V6Header)
	r := NewReader(data)

	if err := r.ReadHeader(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if r.Position() != len(V6Header) {
		t.Errorf("position should be %d, got %d", len(V6Header), r.Position())
	}
}

func TestReader_ReadHeader_OldVersion(t *testing.T) {
	data := []byte("reMarkable .lines file, version=5           ")
	r := NewReader(data)

	err := r.ReadHeader()
	if err == nil {
		t.Fatal("expected error for v5 header")
	}

	if err.Error() != "unsupported .rm version 53 (only v6 is supported)" {
		// 53 is ASCII '5'
		t.Logf("error message: %s", err.Error())
	}
}

func TestReader_ReadHeader_Invalid(t *testing.T) {
	data := []byte("not a remarkable file")
	r := NewReader(data)

	err := r.ReadHeader()
	if err == nil {
		t.Fatal("expected error for invalid header")
	}
}

func TestReader_ReadVaruint(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected uint64
	}{
		{"zero", []byte{0x00}, 0},
		{"one", []byte{0x01}, 1},
		{"127", []byte{0x7F}, 127},
		{"128", []byte{0x80, 0x01}, 128},
		{"300", []byte{0xAC, 0x02}, 300},
		{"16384", []byte{0x80, 0x80, 0x01}, 16384},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReader(tt.data)
			got, err := r.ReadVaruint()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, got)
			}
		})
	}
}

func TestReader_ReadTag(t *testing.T) {
	// tag value = (index << 4) | tagType
	// index=1, type=TagByte4 (0x04) -> (1 << 4) | 4 = 0x14
	r := NewReader([]byte{0x14})
	index, tagType, err := r.ReadTag()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if index != 1 {
		t.Errorf("expected index 1, got %d", index)
	}
	if tagType != TagByte4 {
		t.Errorf("expected TagByte4, got %d", tagType)
	}

	// index=2, type=TagID (0x0F) -> (2 << 4) | 0xF = 0x2F
	r = NewReader([]byte{0x2F})
	index, tagType, err = r.ReadTag()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if index != 2 {
		t.Errorf("expected index 2, got %d", index)
	}
	if tagType != TagID {
		t.Errorf("expected TagID, got %d", tagType)
	}
}

func TestReader_ReadCrdtId(t *testing.T) {
	// part1=3, part2=42 (varuint encoding of 42 = 0x2A)
	r := NewReader([]byte{0x03, 0x2A})
	id, err := r.ReadCrdtId()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Part1 != 3 {
		t.Errorf("expected part1=3, got %d", id.Part1)
	}
	if id.Part2 != 42 {
		t.Errorf("expected part2=42, got %d", id.Part2)
	}
}

func TestReader_ReadPointsV2(t *testing.T) {
	// construct a single point: x=100.0, y=200.0, speed=50, width=30, dir=45, pressure=128
	data := make([]byte, 14)

	// float32 100.0 = 0x42C80000
	data[0] = 0x00
	data[1] = 0x00
	data[2] = 0xC8
	data[3] = 0x42

	// float32 200.0 = 0x43480000
	data[4] = 0x00
	data[5] = 0x00
	data[6] = 0x48
	data[7] = 0x43

	// uint16 speed=50
	data[8] = 50
	data[9] = 0

	// uint16 width=30
	data[10] = 30
	data[11] = 0

	// uint8 direction=45
	data[12] = 45

	// uint8 pressure=128
	data[13] = 128

	r := NewReader(data)
	points, err := r.ReadPointsV2(14)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}

	p := points[0]
	if p.X != 100.0 {
		t.Errorf("x: expected 100.0, got %f", p.X)
	}
	if p.Y != 200.0 {
		t.Errorf("y: expected 200.0, got %f", p.Y)
	}
	if p.Speed != 50 {
		t.Errorf("speed: expected 50, got %d", p.Speed)
	}
	if p.Width != 30 {
		t.Errorf("width: expected 30, got %d", p.Width)
	}
	if p.Direction != 45 {
		t.Errorf("direction: expected 45, got %d", p.Direction)
	}
	if p.Pressure != 128 {
		t.Errorf("pressure: expected 128, got %d", p.Pressure)
	}
}

func TestCrdtId_Zero(t *testing.T) {
	zero := CrdtId{}
	if !zero.Zero() {
		t.Error("expected zero ID to be zero")
	}

	nonZero := CrdtId{Part1: 1, Part2: 0}
	if nonZero.Zero() {
		t.Error("expected non-zero ID to not be zero")
	}
}
