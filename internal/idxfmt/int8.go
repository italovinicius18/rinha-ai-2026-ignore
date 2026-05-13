package idxfmt

import (
	"bufio"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
)

// QuantizeFloat64 maps a float64 to a uint8 storage value for FormatInt8.
//
// [0, 1] → [0, 254] (linear, rounded).
// Negative values (the -1 sentinel used when last_transaction is nil) → 255.
//
// 255 is reserved so that null-last-transaction entries are distinguishable
// from valid values at the far end of the [0,1] range, and so that the uint32
// squared-distance metric naturally separates null entries from valid ones.
func QuantizeFloat64(v float64) uint8 {
	if v < 0 {
		return 255
	}
	q := v*254.0 + 0.5 // round to nearest
	if q >= 255 {
		return 254
	}
	return uint8(q)
}

// DequantizeUint8 converts a stored uint8 back to float64.
// 255 (null sentinel) → -1; [0,254] → [0,1].
func DequantizeUint8(b uint8) float64 {
	if b == 255 {
		return -1
	}
	return float64(b) / 254.0
}

// Int8BodySize returns the byte count of the body for a FormatInt8 file with n entries.
func Int8BodySize(n int) int {
	return n*Dim + (n+7)/8
}

// Int8Writer streams entries to a FormatInt8 file. Each entry's vector is
// quantized from float64 to uint8 before writing.
type Int8Writer struct {
	f       io.WriteSeeker
	bw      *bufio.Writer
	crc     hash.Hash32
	n       uint32
	labels  []byte
	scratch [Dim]byte
	closed  bool
}

// NewInt8Writer reserves the header at the start of f and returns a writer.
func NewInt8Writer(f io.WriteSeeker) (*Int8Writer, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var zero [HeaderSize]byte
	if _, err := f.Write(zero[:]); err != nil {
		return nil, err
	}
	return &Int8Writer{
		f:      f,
		bw:     bufio.NewWriterSize(f, 1<<20),
		crc:    crc32.NewIEEE(),
		labels: make([]byte, 0, 1<<14),
	}, nil
}

// Add appends one entry. vec is quantized from float64 to uint8.
func (w *Int8Writer) Add(v [Dim]float64, fraud bool) error {
	if w.closed {
		return fmt.Errorf("idxfmt: Add after Close")
	}
	for i := 0; i < Dim; i++ {
		w.scratch[i] = QuantizeFloat64(v[i])
	}
	if _, err := w.bw.Write(w.scratch[:]); err != nil {
		return err
	}
	w.crc.Write(w.scratch[:])

	if int(w.n)%8 == 0 {
		w.labels = append(w.labels, 0)
	}
	if fraud {
		w.labels[len(w.labels)-1] |= 1 << (w.n % 8)
	}
	w.n++
	return nil
}

// Close appends the label block, flushes, and writes the real header.
func (w *Int8Writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true

	if _, err := w.bw.Write(w.labels); err != nil {
		return err
	}
	w.crc.Write(w.labels)
	if err := w.bw.Flush(); err != nil {
		return err
	}

	h := Header{
		Version: Version,
		Format:  FormatInt8,
		Count:   w.n,
		Dim:     Dim,
		CRC32:   w.crc.Sum32(),
	}
	hb := h.Marshal()
	if _, err := w.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := w.f.Write(hb[:])
	return err
}
