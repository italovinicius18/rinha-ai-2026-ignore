package idxfmt

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"math"
)

// FlatWriter streams entries to a FormatFlat32 file. Vectors are written
// straight through (buffered) as they arrive; labels are bit-packed into an
// in-memory slice and appended after the vector block on Close. That keeps the
// on-disk layout "vectors ‖ labels" while still letting Add be O(1) — no need
// to seek mid-stream.
type FlatWriter struct {
	f       io.WriteSeeker
	bw      *bufio.Writer
	crc     hash.Hash32
	n       uint32
	labels  []byte // bit-packed; bit (i mod 8) of byte (i / 8) holds entry i
	scratch [Dim * 4]byte
	closed  bool
}

// NewFlatWriter reserves the header at the start of f and returns a writer.
func NewFlatWriter(f io.WriteSeeker) (*FlatWriter, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var zero [HeaderSize]byte
	if _, err := f.Write(zero[:]); err != nil {
		return nil, err
	}
	return &FlatWriter{
		f:      f,
		bw:     bufio.NewWriterSize(f, 1<<20),
		crc:    crc32.NewIEEE(),
		labels: make([]byte, 0, 1<<14),
	}, nil
}

// Add appends one entry to the file.
func (w *FlatWriter) Add(vec [Dim]float64, fraud bool) error {
	if w.closed {
		return fmt.Errorf("idxfmt: Add after Close")
	}
	for i := 0; i < Dim; i++ {
		binary.LittleEndian.PutUint32(w.scratch[i*4:(i+1)*4], math.Float32bits(float32(vec[i])))
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

// Close appends the labels block, flushes the buffered writer, then seeks
// back and writes the real header.
func (w *FlatWriter) Close() error {
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
		Format:  FormatFlat32,
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

// BodySize returns the byte length of body for a FLAT32 file with n entries.
func BodySize(n int) int {
	return n*Dim*4 + (n+7)/8
}

// FlatReader is a buffer-backed read-only view of a FormatFlat32 file.
// (Runtime mmap reader will live elsewhere; this is for tooling + tests.)
type FlatReader struct {
	data     []byte
	h        Header
	vecOff   int
	labelOff int
}

// OpenFlat parses data (the entire file) and verifies the CRC.
func OpenFlat(data []byte) (*FlatReader, error) {
	h, err := ParseHeader(data)
	if err != nil {
		return nil, err
	}
	if h.Format != FormatFlat32 {
		return nil, fmt.Errorf("idxfmt: format %d is not FLAT32", h.Format)
	}
	need := HeaderSize + BodySize(int(h.Count))
	if len(data) < need {
		return nil, fmt.Errorf("idxfmt: file too short (have %d, need %d)", len(data), need)
	}
	body := data[HeaderSize:need]
	if got := crc32.ChecksumIEEE(body); got != h.CRC32 {
		return nil, fmt.Errorf("idxfmt: CRC mismatch (got %#08x, want %#08x)", got, h.CRC32)
	}
	return &FlatReader{
		data:     data[:need],
		h:        h,
		vecOff:   HeaderSize,
		labelOff: HeaderSize + int(h.Count)*Dim*4,
	}, nil
}

func (r *FlatReader) Header() Header { return r.h }
func (r *FlatReader) Count() int     { return int(r.h.Count) }

// Vector returns entry i as a [14]float32 copy.
func (r *FlatReader) Vector(i int) [Dim]float32 {
	off := r.vecOff + i*Dim*4
	var v [Dim]float32
	for j := 0; j < Dim; j++ {
		v[j] = math.Float32frombits(binary.LittleEndian.Uint32(r.data[off+j*4 : off+(j+1)*4]))
	}
	return v
}

// IsFraud reports whether entry i is labeled fraud.
func (r *FlatReader) IsFraud(i int) bool {
	return r.data[r.labelOff+i/8]&(1<<(i%8)) != 0
}
