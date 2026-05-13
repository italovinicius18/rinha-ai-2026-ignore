package idxfmt

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// sample mirrors the structure of resources/example-references.json:
// the first row is the legit example vector from DETECTION_RULES.md,
// the second is the fraud example, and we add an odd count to exercise
// the partial-label-byte path.
var sample = []struct {
	v     [Dim]float64
	fraud bool
}{
	{[Dim]float64{0.01, 0.0833, 0.05, 0.8261, 0.1667, -1, -1, 0.0432, 0.25, 0, 1, 0, 0.2, 0.0416}, false},
	{[Dim]float64{0.5796, 0.9167, 1, 0.0435, 0, 0.0056, 0.4394, 0.4598, 0.4, 1, 0, 1, 0.85, 0.0032}, true},
	{[Dim]float64{0.0035, 0.1667, 0.05, 0.4348, 0.6667, 0.1278, 0.0008, 0.017, 0.1, 0, 1, 0, 0.2, 0.02}, false},
	{[Dim]float64{0.9506, 0.8333, 1.0, 0.2174, 0.8333, -1, -1, 0.9523, 1.0, 0, 1, 1, 0.75, 0.0055}, true},
	{[Dim]float64{0.0041, 0.1667, 0.05, 0.7826, 0.3333, -1, -1, 0.0292, 0.15, 0, 1, 0, 0.15, 0.006}, false},
}

func writeSample(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "idx.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w, err := NewFlatWriter(f)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sample {
		if err := w.Add(s.v, s.fraud); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFlatRoundTrip(t *testing.T) {
	path := writeSample(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	wantBody := BodySize(len(sample))
	if got := len(data); got != HeaderSize+wantBody {
		t.Errorf("file size: got %d, want %d", got, HeaderSize+wantBody)
	}

	r, err := OpenFlat(data)
	if err != nil {
		t.Fatal(err)
	}
	if r.Count() != len(sample) {
		t.Errorf("count: got %d, want %d", r.Count(), len(sample))
	}
	if r.Header().Format != FormatFlat32 {
		t.Errorf("format: got %d, want %d", r.Header().Format, FormatFlat32)
	}

	for i, s := range sample {
		v := r.Vector(i)
		for j := 0; j < Dim; j++ {
			got := float64(v[j])
			if math.Abs(got-s.v[j]) > 1e-5 {
				t.Errorf("entry %d dim %d: got %v, want %v", i, j, got, s.v[j])
			}
		}
		if got := r.IsFraud(i); got != s.fraud {
			t.Errorf("entry %d label: got %v, want %v", i, got, s.fraud)
		}
	}
}

func TestOpenFlat_BadMagic(t *testing.T) {
	var b [HeaderSize]byte
	copy(b[:], "NOPE2026")
	if _, err := OpenFlat(b[:]); err == nil {
		t.Error("expected error on bad magic")
	}
}

func TestOpenFlat_BadCRC(t *testing.T) {
	path := writeSample(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Flip a body byte → CRC must fail.
	data[HeaderSize] ^= 0xff
	if _, err := OpenFlat(data); err == nil {
		t.Error("expected CRC error after tampering with body")
	}
}

func TestOpenFlat_TooShort(t *testing.T) {
	path := writeSample(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	short := data[:len(data)-1]
	if _, err := OpenFlat(short); err == nil {
		t.Error("expected error when body is truncated")
	}
}

func TestPartialLabelByte(t *testing.T) {
	// 13 entries → label bytes = ceil(13/8) = 2 (one full + one with 5 bits).
	path := filepath.Join(t.TempDir(), "odd.bin")
	f, _ := os.Create(path)
	w, _ := NewFlatWriter(f)
	pattern := []bool{true, false, false, true, false, true, true, false, true, true, true, false, true}
	var z [Dim]float64
	for _, fraud := range pattern {
		if err := w.Add(z, fraud); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	f.Close()

	data, _ := os.ReadFile(path)
	r, err := OpenFlat(data)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range pattern {
		if got := r.IsFraud(i); got != want {
			t.Errorf("entry %d: got %v, want %v", i, got, want)
		}
	}
}
