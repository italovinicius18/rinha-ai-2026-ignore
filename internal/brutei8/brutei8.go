// Package brutei8 implements brute-force k=5 KNN over a FormatInt8 index.
//
// Vectors are stored as uint8 (quantized from float64 by idxfmt.QuantizeFloat64).
// Distance is uint32 squared Euclidean over int16 differences, so there is no
// overflow risk: 14 × 255² = 910 350, well within uint32.
//
// The 4× smaller index (~42 MB vs ~168 MB) means faster sequential scans and
// much lower page-fault pressure under cgroup memory limits.
package brutei8

import (
	"fmt"
	"math"
	"unsafe"

	"rinha2026/solution/internal/idxfmt"
)

const K = 5

// Index is a read-only view over a FormatInt8 index buffer.
type Index struct {
	h       idxfmt.Header
	vecBase []byte
	lblBase []byte
	count   int
}

// Open validates the header and returns a queryable index over data.
func Open(data []byte) (*Index, error) {
	h, err := idxfmt.ParseHeader(data)
	if err != nil {
		return nil, err
	}
	if h.Format != idxfmt.FormatInt8 {
		return nil, fmt.Errorf("brutei8: format %d is not FormatInt8", h.Format)
	}
	need := idxfmt.HeaderSize + idxfmt.Int8BodySize(int(h.Count))
	if len(data) < need {
		return nil, fmt.Errorf("brutei8: buffer too short (have %d, need %d)", len(data), need)
	}
	vecStart := idxfmt.HeaderSize
	vecEnd := idxfmt.HeaderSize + int(h.Count)*idxfmt.Dim
	return &Index{
		h:       h,
		vecBase: data[vecStart:vecEnd],
		lblBase: data[vecEnd:need],
		count:   int(h.Count),
	}, nil
}

// Count returns the number of reference entries.
func (ix *Index) Count() int { return ix.count }

// vec returns a pointer to entry i's [14]uint8. Caller must not mutate.
func (ix *Index) vec(i int) *[idxfmt.Dim]uint8 {
	return (*[idxfmt.Dim]uint8)(unsafe.Pointer(&ix.vecBase[i*idxfmt.Dim]))
}

// IsFraud reports whether entry i is labeled fraud.
func (ix *Index) IsFraud(i int) bool {
	return ix.lblBase[i/8]&(1<<(i%8)) != 0
}

// SearchKNN returns the number of fraud labels (0..K) among the K nearest
// neighbors of q. q must be pre-quantized (idxfmt.QuantizeFloat64 per dim).
//
//go:nosplit
func (ix *Index) SearchKNN(q *[idxfmt.Dim]uint8) uint8 {
	var topD [K]uint32
	var topI [K]int32
	for i := 0; i < K; i++ {
		topD[i] = math.MaxUint32
		topI[i] = -1
	}

	for i := 0; i < ix.count; i++ {
		v := ix.vec(i)
		// Unrolled 14-dim squared uint8 distance (computed as int32 diffs).
		d0 := int32(v[0]) - int32(q[0])
		d1 := int32(v[1]) - int32(q[1])
		d2 := int32(v[2]) - int32(q[2])
		d3 := int32(v[3]) - int32(q[3])
		d4 := int32(v[4]) - int32(q[4])
		d5 := int32(v[5]) - int32(q[5])
		d6 := int32(v[6]) - int32(q[6])
		d7 := int32(v[7]) - int32(q[7])
		d8 := int32(v[8]) - int32(q[8])
		d9 := int32(v[9]) - int32(q[9])
		d10 := int32(v[10]) - int32(q[10])
		d11 := int32(v[11]) - int32(q[11])
		d12 := int32(v[12]) - int32(q[12])
		d13 := int32(v[13]) - int32(q[13])
		dist := uint32(d0*d0 + d1*d1 + d2*d2 + d3*d3 +
			d4*d4 + d5*d5 + d6*d6 + d7*d7 +
			d8*d8 + d9*d9 + d10*d10 + d11*d11 +
			d12*d12 + d13*d13)

		if dist < topD[K-1] {
			pos := K - 1
			for pos > 0 && topD[pos-1] > dist {
				topD[pos] = topD[pos-1]
				topI[pos] = topI[pos-1]
				pos--
			}
			topD[pos] = dist
			topI[pos] = int32(i)
		}
	}

	var nFraud uint8
	for k := 0; k < K; k++ {
		idx := int(topI[k])
		if idx < 0 {
			continue
		}
		if ix.IsFraud(idx) {
			nFraud++
		}
	}
	return nFraud
}
