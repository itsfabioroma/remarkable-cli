package rm

import (
	"math"
	"testing"
)

func TestVaruintRoundTrip(t *testing.T) {
	cases := []uint64{0, 1, 127, 128, 300, 16384}

	for _, v := range cases {
		w := NewWriter()
		w.WriteVaruint(v)

		r := NewReader(w.Bytes())
		got, err := r.ReadVaruint()
		if err != nil {
			t.Fatalf("ReadVaruint(%d): %v", v, err)
		}
		if got != v {
			t.Errorf("varuint round-trip: wrote %d, got %d", v, got)
		}
	}
}

func TestFloat32RoundTrip(t *testing.T) {
	cases := []float32{0, 1.5, -3.14, math.MaxFloat32, math.SmallestNonzeroFloat32}

	for _, v := range cases {
		w := NewWriter()
		w.WriteFloat32(v)

		r := NewReader(w.Bytes())
		got, err := r.ReadFloat32()
		if err != nil {
			t.Fatalf("ReadFloat32(%v): %v", v, err)
		}
		if got != v {
			t.Errorf("float32 round-trip: wrote %v, got %v", v, got)
		}
	}
}

func TestCrdtIdRoundTrip(t *testing.T) {
	cases := []CrdtId{
		{0, 0},
		{1, 42},
		{255, 16384},
	}

	for _, id := range cases {
		w := NewWriter()
		w.WriteCrdtId(id)

		r := NewReader(w.Bytes())
		got, err := r.ReadCrdtId()
		if err != nil {
			t.Fatalf("ReadCrdtId(%+v): %v", id, err)
		}
		if got != id {
			t.Errorf("CrdtId round-trip: wrote %+v, got %+v", id, got)
		}
	}
}

func TestPointsV2RoundTrip(t *testing.T) {
	points := []Point{
		{X: 100.5, Y: 200.25, Speed: 300, Width: 40, Direction: 90, Pressure: 128},
		{X: -50.0, Y: 0, Speed: 0, Width: 65535, Direction: 255, Pressure: 0},
	}

	w := NewWriter()
	w.WritePointsV2(points)

	// 14 bytes per point
	r := NewReader(w.Bytes())
	got, err := r.ReadPointsV2(len(points) * 14)
	if err != nil {
		t.Fatalf("ReadPointsV2: %v", err)
	}

	if len(got) != len(points) {
		t.Fatalf("expected %d points, got %d", len(points), len(got))
	}

	for i := range points {
		if got[i] != points[i] {
			t.Errorf("point[%d]: wrote %+v, got %+v", i, points[i], got[i])
		}
	}
}

func TestHeaderRoundTrip(t *testing.T) {
	w := NewWriter()
	w.WriteHeader()

	r := NewReader(w.Bytes())
	if err := r.ReadHeader(); err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}
}

func TestTagRoundTrip(t *testing.T) {
	cases := []struct {
		index uint64
		tag   TagType
	}{
		{0, TagByte1},
		{1, TagByte4},
		{5, TagLength4},
		{15, TagID},
	}

	for _, c := range cases {
		w := NewWriter()
		w.WriteTag(c.index, c.tag)

		r := NewReader(w.Bytes())
		idx, tag, err := r.ReadTag()
		if err != nil {
			t.Fatalf("ReadTag(%d, %d): %v", c.index, c.tag, err)
		}
		if idx != c.index || tag != c.tag {
			t.Errorf("tag round-trip: wrote (%d,%d), got (%d,%d)", c.index, c.tag, idx, tag)
		}
	}
}
