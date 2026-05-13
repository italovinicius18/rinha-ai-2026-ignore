package idxfmt

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"math"
)

// IVF file layout (all integers little-endian):
//
//	header        64 B    (magic, version, Format=3, Count=total_vectors, CRC32=0)
//	nCentroids     4 B    uint32
//	centroids    C×56 B   C × Dim × float32 (row-major)
//	sizes        C× 4 B   uint32 per-cluster vector count
//	offsets    (C+1)×8 B  uint64 absolute file byte offsets for each cluster's
//	                      data; offsets[C] = one-past-end sentinel
//	cluster_data         for c in 0..C-1:
//	                       sizes[c]×14 bytes (uint8 int8-quantized vectors)
//	                       ceil(sizes[c]/8) bytes (bit-packed labels)
//
// CRC32 is stored as 0 — the IVF index is built offline by a trusted tool and
// large enough that recomputing CRC at load time is expensive.

// IVFMetaSize returns the fixed-overhead byte count before the first cluster.
func IVFMetaSize(nCentroids int) int {
	return 4 + nCentroids*Dim*4 + nCentroids*4 + (nCentroids+1)*8
}

// IVFClusterDataSize returns the byte size of one int8 cluster's on-disk data.
func IVFClusterDataSize(count int) int {
	return count*Dim + (count+7)/8
}

// IVFClusterDataSizeF32 returns the byte size of one float32 cluster's on-disk data.
func IVFClusterDataSizeF32(count int) int {
	return count*Dim*4 + (count+7)/8
}

// IVFWriter builds a FormatIVF or FormatIVF_F32 file.
//
// sizes must be provided at construction time (the builder knows cluster sizes
// after k-means assignment). AddCluster (int8) or AddClusterF32 must then be
// called exactly once per cluster, in order 0 … nCentroids-1.
type IVFWriter struct {
	f          io.WriteSeeker
	bw         *bufio.Writer
	crc        interface {
		Write([]byte) (int, error)
		Sum32() uint32
	}
	nCentroids int
	sizes      []uint32
	offsets    []uint64 // nCentroids+1 absolute byte offsets
	totalVecs  uint32
	clusterIdx int
	isF32      bool
	closed     bool
}

// NewIVFWriter writes the header + metadata for a FormatIVF (int8 clusters) file.
func NewIVFWriter(f io.WriteSeeker, centroids []float32, sizes []uint32) (*IVFWriter, error) {
	return newIVFWriter(f, centroids, sizes, false)
}

// NewIVFWriterF32 writes the header + metadata for a FormatIVF_F32 (float32 clusters) file.
func NewIVFWriterF32(f io.WriteSeeker, centroids []float32, sizes []uint32) (*IVFWriter, error) {
	return newIVFWriter(f, centroids, sizes, true)
}

func newIVFWriter(f io.WriteSeeker, centroids []float32, sizes []uint32, isF32 bool) (*IVFWriter, error) {
	nC := len(centroids) / Dim
	if len(centroids) != nC*Dim {
		return nil, fmt.Errorf("idxfmt: centroids length %d not multiple of Dim=%d", len(centroids), Dim)
	}
	if len(sizes) != nC {
		return nil, fmt.Errorf("idxfmt: len(sizes)=%d != nCentroids=%d", len(sizes), nC)
	}

	// Compute total vectors and per-cluster byte offsets.
	offsets := make([]uint64, nC+1)
	dataStart := int64(HeaderSize) + int64(IVFMetaSize(nC))
	var pos uint64
	var total uint32
	for c, s := range sizes {
		offsets[c] = uint64(dataStart) + pos
		if isF32 {
			pos += uint64(IVFClusterDataSizeF32(int(s)))
		} else {
			pos += uint64(IVFClusterDataSize(int(s)))
		}
		total += s
	}
	offsets[nC] = uint64(dataStart) + pos

	// Write header placeholder.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var zero [HeaderSize]byte
	if _, err := f.Write(zero[:]); err != nil {
		return nil, err
	}

	crc := crc32.NewIEEE()
	bw := bufio.NewWriterSize(f, 1<<20)

	// nCentroids
	var buf4 [4]byte
	binary.LittleEndian.PutUint32(buf4[:], uint32(nC))
	bw.Write(buf4[:])
	crc.Write(buf4[:])

	// Centroids
	for _, v := range centroids {
		binary.LittleEndian.PutUint32(buf4[:], math.Float32bits(v))
		bw.Write(buf4[:])
		crc.Write(buf4[:])
	}

	// Sizes
	for _, s := range sizes {
		binary.LittleEndian.PutUint32(buf4[:], s)
		bw.Write(buf4[:])
		crc.Write(buf4[:])
	}

	// Offsets
	var buf8 [8]byte
	for _, o := range offsets {
		binary.LittleEndian.PutUint64(buf8[:], o)
		bw.Write(buf8[:])
		crc.Write(buf8[:])
	}

	if err := bw.Flush(); err != nil {
		return nil, err
	}

	return &IVFWriter{
		f:          f,
		bw:         bufio.NewWriterSize(f, 4<<20),
		crc:        crc,
		nCentroids: nC,
		sizes:      sizes,
		offsets:    offsets,
		totalVecs:  total,
		isF32:      isF32,
	}, nil
}

// AddCluster writes one cluster's data. vecs must be sizes[c]×Dim uint8.
// labels must be sizes[c] booleans.
func (w *IVFWriter) AddCluster(vecs []byte, labels []bool) error {
	if w.closed {
		return fmt.Errorf("idxfmt: AddCluster after Close")
	}
	c := w.clusterIdx
	if c >= w.nCentroids {
		return fmt.Errorf("idxfmt: too many clusters")
	}
	count := int(w.sizes[c])
	if len(vecs) != count*Dim {
		return fmt.Errorf("idxfmt: cluster %d: want %d vec bytes, got %d", c, count*Dim, len(vecs))
	}
	if len(labels) != count {
		return fmt.Errorf("idxfmt: cluster %d: want %d labels, got %d", c, count, len(labels))
	}

	if _, err := w.bw.Write(vecs); err != nil {
		return err
	}
	w.crc.Write(vecs)

	lblBytes := make([]byte, (count+7)/8)
	for i, fraud := range labels {
		if fraud {
			lblBytes[i/8] |= 1 << (i % 8)
		}
	}
	if _, err := w.bw.Write(lblBytes); err != nil {
		return err
	}
	w.crc.Write(lblBytes)

	w.clusterIdx++
	return nil
}

// AddClusterF32 writes one cluster's data using float32 vectors.
// vecs must contain sizes[c] contiguous float32 values per dimension (len = sizes[c]*Dim).
// labels must be sizes[c] booleans.
func (w *IVFWriter) AddClusterF32(vecs []float32, labels []bool) error {
	if w.closed {
		return fmt.Errorf("idxfmt: AddClusterF32 after Close")
	}
	c := w.clusterIdx
	if c >= w.nCentroids {
		return fmt.Errorf("idxfmt: too many clusters")
	}
	count := int(w.sizes[c])
	if len(vecs) != count*Dim {
		return fmt.Errorf("idxfmt: cluster %d: want %d float32 values, got %d", c, count*Dim, len(vecs))
	}
	if len(labels) != count {
		return fmt.Errorf("idxfmt: cluster %d: want %d labels, got %d", c, count, len(labels))
	}

	var buf4 [4]byte
	for _, v := range vecs {
		binary.LittleEndian.PutUint32(buf4[:], math.Float32bits(v))
		w.bw.Write(buf4[:])
		w.crc.Write(buf4[:])
	}

	lblBytes := make([]byte, (count+7)/8)
	for i, fraud := range labels {
		if fraud {
			lblBytes[i/8] |= 1 << (i % 8)
		}
	}
	if _, err := w.bw.Write(lblBytes); err != nil {
		return err
	}
	w.crc.Write(lblBytes)

	w.clusterIdx++
	return nil
}

// Close flushes buffered data and writes the real header.
func (w *IVFWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if w.clusterIdx != w.nCentroids {
		return fmt.Errorf("idxfmt: IVFWriter closed after %d/%d clusters", w.clusterIdx, w.nCentroids)
	}
	if err := w.bw.Flush(); err != nil {
		return err
	}

	format := uint16(FormatIVF)
	if w.isF32 {
		format = FormatIVF_F32
	}
	h := Header{
		Version: Version,
		Format:  format,
		Count:   w.totalVecs,
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
