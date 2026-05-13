// Package ivf implements approximate k=5 KNN using an Inverted File Index (IVF).
//
// Supports two formats:
//   - FormatIVF (int8 clusters): fast search (~62 μs nprobe=8 C=512), ~4.3% borderline rate.
//   - FormatIVF_F32 (float32 clusters): accurate search, eliminates int8 quantization error.
//
// At query time:
//  1. Compute squared float32 distance to all C centroids.
//  2. Pick the nprobe nearest centroids.
//  3. Search their cluster vectors (int8 or float32 depending on format).
//  4. Return fraud-label count among the global top-5.
//
// SearchKNNAdaptive: fast int8 pass, brute-force fallback for borderline results.
// For FormatIVF_F32 indexes, SearchKNN alone is accurate enough.
package ivf

import (
	"encoding/binary"
	"fmt"
	"math"
	"unsafe"

	"rinha2026/solution/internal/idxfmt"
)

const K = 5

// Index is a read-only view of a FormatIVF or FormatIVF_F32 file backed by a mmap'd byte slice.
type Index struct {
	nCentroids int
	centroids  []float32 // nCentroids × Dim (parsed copy — small)
	sizes      []uint32  // nCentroids
	offsets    []uint64  // nCentroids+1 absolute byte offsets
	data       []byte    // entire mmap'd file
	totalCount int
	isF32      bool // true for FormatIVF_F32 (float32 cluster vectors)
}

// Open parses the IVF header and metadata from the mmap'd file.
func Open(data []byte) (*Index, error) {
	h, err := idxfmt.ParseHeader(data)
	if err != nil {
		return nil, err
	}
	isF32 := false
	switch h.Format {
	case idxfmt.FormatIVF:
		// int8 cluster vectors
	case idxfmt.FormatIVF_F32:
		isF32 = true
	default:
		return nil, fmt.Errorf("ivf: format %d is not FormatIVF or FormatIVF_F32", h.Format)
	}

	pos := idxfmt.HeaderSize

	// nCentroids
	if len(data) < pos+4 {
		return nil, fmt.Errorf("ivf: file too short for nCentroids")
	}
	nC := int(binary.LittleEndian.Uint32(data[pos:]))
	pos += 4

	// Centroids
	centBytes := nC * idxfmt.Dim * 4
	if len(data) < pos+centBytes {
		return nil, fmt.Errorf("ivf: file too short for centroids")
	}
	centroids := make([]float32, nC*idxfmt.Dim)
	for i := range centroids {
		centroids[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[pos+i*4:]))
	}
	pos += centBytes

	// Sizes
	if len(data) < pos+nC*4 {
		return nil, fmt.Errorf("ivf: file too short for sizes")
	}
	sizes := make([]uint32, nC)
	for i := range sizes {
		sizes[i] = binary.LittleEndian.Uint32(data[pos+i*4:])
	}
	pos += nC * 4

	// Offsets
	if len(data) < pos+(nC+1)*8 {
		return nil, fmt.Errorf("ivf: file too short for offsets")
	}
	offsets := make([]uint64, nC+1)
	for i := range offsets {
		offsets[i] = binary.LittleEndian.Uint64(data[pos+i*8:])
	}

	return &Index{
		nCentroids: nC,
		centroids:  centroids,
		sizes:      sizes,
		offsets:    offsets,
		data:       data,
		totalCount: int(h.Count),
		isF32:      isF32,
	}, nil
}

// Count returns the total number of indexed vectors.
func (ix *Index) Count() int { return ix.totalCount }

// NCentroids returns the number of IVF clusters.
func (ix *Index) NCentroids() int { return ix.nCentroids }

// maxNprobeStack is the maximum nprobe for which centroid buffers are
// kept on the stack (avoiding heap allocs on the hot path).
const maxNprobeStack = 64

// SearchKNN returns the fraud-label count (0..K) among the K nearest
// neighbors of q, searching the nprobe nearest clusters.
func (ix *Index) SearchKNN(q *[idxfmt.Dim]float32, nprobe int) uint8 {
	if nprobe > ix.nCentroids {
		nprobe = ix.nCentroids
	}

	// Find the nprobe nearest centroids.
	// Use stack-allocated buffers to avoid heap allocs on every request.
	var topCDbuf [maxNprobeStack]float32
	var topCIbuf [maxNprobeStack]int
	var topCD []float32
	var topCI []int
	if nprobe <= maxNprobeStack {
		topCD = topCDbuf[:nprobe]
		topCI = topCIbuf[:nprobe]
	} else {
		topCD = make([]float32, nprobe)
		topCI = make([]int, nprobe)
	}
	for i := range topCD {
		topCD[i] = math.MaxFloat32
		topCI[i] = -1
	}
	for c := 0; c < ix.nCentroids; c++ {
		cent := ix.centroids[c*idxfmt.Dim : (c+1)*idxfmt.Dim]
		var d float32
		for j := 0; j < idxfmt.Dim; j++ {
			dj := cent[j] - q[j]
			d += dj * dj
		}
		if d < topCD[nprobe-1] {
			pos := nprobe - 1
			for pos > 0 && topCD[pos-1] > d {
				topCD[pos] = topCD[pos-1]
				topCI[pos] = topCI[pos-1]
				pos--
			}
			topCD[pos] = d
			topCI[pos] = c
		}
	}

	if ix.isF32 {
		return ix.searchAndCountF32(q, topCI)
	}
	// int8 path: quantize query once, then search.
	var qI8 [idxfmt.Dim]uint8
	for j := 0; j < idxfmt.Dim; j++ {
		qI8[j] = idxfmt.QuantizeFloat64(float64(q[j]))
	}
	return ix.searchAndCount(&qI8, topCI)
}

// SearchKNNAdaptive runs a two-pass search (int8 format only).
//
// Pass 1: search the nprobe nearest clusters (~62 μs for nprobe=8 C=512).
// If the result is confident (0 or 5 frauds out of K), return immediately.
// Pass 2 (borderline only, nFraud==2 or nFraud==3): run BruteForce over all
// clusters to eliminate ANN + quantization error at the cost of ~4 ms.
//
// For FormatIVF_F32 indexes, use SearchKNN directly (no quantization error).
func (ix *Index) SearchKNNAdaptive(q *[idxfmt.Dim]float32, nprobe int) uint8 {
	if ix.isF32 {
		return ix.SearchKNN(q, nprobe)
	}

	nFraud := ix.SearchKNN(q, nprobe)

	// Unanimous results (0 or 5) are reliable enough to skip the fallback.
	if nFraud == 2 || nFraud == 3 {
		var qI8 [idxfmt.Dim]uint8
		for j := 0; j < idxfmt.Dim; j++ {
			qI8[j] = idxfmt.QuantizeFloat64(float64(q[j]))
		}
		nFraud = ix.BruteForce(&qI8)
	}
	return nFraud
}

// BruteForce scans ALL clusters (equivalent to brute-force int8 KNN).
// Only valid for FormatIVF (int8) indexes.
func (ix *Index) BruteForce(qI8 *[idxfmt.Dim]uint8) uint8 {
	all := make([]int, ix.nCentroids)
	for i := range all {
		all[i] = i
	}
	return ix.searchAndCount(qI8, all)
}

// searchAndCount is the corrected implementation: searches each cluster and
// accumulates global top-K while carrying the fraud bit.
func (ix *Index) searchAndCount(qI8 *[idxfmt.Dim]uint8, clusters []int) uint8 {
	// Global top-K: (distance, isFraud).
	var topD [K]uint32
	var topF [K]bool
	for i := range topD {
		topD[i] = math.MaxUint32
	}

	for _, c := range clusters {
		if c < 0 {
			continue
		}
		count := int(ix.sizes[c])
		if count == 0 {
			continue
		}
		off := ix.offsets[c]
		vecData := ix.data[off : off+uint64(count*idxfmt.Dim)]
		lblOff := off + uint64(count*idxfmt.Dim)

		for i := 0; i < count; i++ {
			v := (*[idxfmt.Dim]uint8)(unsafe.Pointer(&vecData[i*idxfmt.Dim]))
			d0 := int32(v[0]) - int32(qI8[0])
			d1 := int32(v[1]) - int32(qI8[1])
			d2 := int32(v[2]) - int32(qI8[2])
			d3 := int32(v[3]) - int32(qI8[3])
			d4 := int32(v[4]) - int32(qI8[4])
			d5 := int32(v[5]) - int32(qI8[5])
			d6 := int32(v[6]) - int32(qI8[6])
			d7 := int32(v[7]) - int32(qI8[7])
			d8 := int32(v[8]) - int32(qI8[8])
			d9 := int32(v[9]) - int32(qI8[9])
			d10 := int32(v[10]) - int32(qI8[10])
			d11 := int32(v[11]) - int32(qI8[11])
			d12 := int32(v[12]) - int32(qI8[12])
			d13 := int32(v[13]) - int32(qI8[13])
			dist := uint32(d0*d0 + d1*d1 + d2*d2 + d3*d3 +
				d4*d4 + d5*d5 + d6*d6 + d7*d7 +
				d8*d8 + d9*d9 + d10*d10 + d11*d11 +
				d12*d12 + d13*d13)

			if dist < topD[K-1] {
				fraud := ix.data[lblOff+uint64(i/8)]&(1<<(i%8)) != 0
				pos := K - 1
				for pos > 0 && topD[pos-1] > dist {
					topD[pos] = topD[pos-1]
					topF[pos] = topF[pos-1]
					pos--
				}
				topD[pos] = dist
				topF[pos] = fraud
			}
		}
	}

	var nFraud uint8
	for k := 0; k < K; k++ {
		if topD[k] == math.MaxUint32 {
			continue
		}
		if topF[k] {
			nFraud++
		}
	}
	return nFraud
}

// searchAndCountF32 is the float32-cluster variant of searchAndCount.
// cluster data layout: count×Dim float32 values, then ceil(count/8) label bytes.
func (ix *Index) searchAndCountF32(q *[idxfmt.Dim]float32, clusters []int) uint8 {
	var topD [K]float32
	var topF [K]bool
	for i := range topD {
		topD[i] = math.MaxFloat32
	}

	for _, c := range clusters {
		if c < 0 {
			continue
		}
		count := int(ix.sizes[c])
		if count == 0 {
			continue
		}
		off := ix.offsets[c]
		vecBytes := ix.data[off : off+uint64(count*idxfmt.Dim*4)]
		lblOff := off + uint64(count*idxfmt.Dim*4)

		for i := 0; i < count; i++ {
			v := (*[idxfmt.Dim]float32)(unsafe.Pointer(&vecBytes[i*idxfmt.Dim*4]))
			d0 := v[0] - q[0]
			d1 := v[1] - q[1]
			d2 := v[2] - q[2]
			d3 := v[3] - q[3]
			d4 := v[4] - q[4]
			d5 := v[5] - q[5]
			d6 := v[6] - q[6]
			d7 := v[7] - q[7]
			d8 := v[8] - q[8]
			d9 := v[9] - q[9]
			d10 := v[10] - q[10]
			d11 := v[11] - q[11]
			d12 := v[12] - q[12]
			d13 := v[13] - q[13]
			dist := d0*d0 + d1*d1 + d2*d2 + d3*d3 +
				d4*d4 + d5*d5 + d6*d6 + d7*d7 +
				d8*d8 + d9*d9 + d10*d10 + d11*d11 +
				d12*d12 + d13*d13

			if dist < topD[K-1] {
				fraud := ix.data[lblOff+uint64(i/8)]&(1<<(i%8)) != 0
				pos := K - 1
				for pos > 0 && topD[pos-1] > dist {
					topD[pos] = topD[pos-1]
					topF[pos] = topF[pos-1]
					pos--
				}
				topD[pos] = dist
				topF[pos] = fraud
			}
		}
	}

	var nFraud uint8
	for k := 0; k < K; k++ {
		if topD[k] == math.MaxFloat32 {
			continue
		}
		if topF[k] {
			nFraud++
		}
	}
	return nFraud
}
