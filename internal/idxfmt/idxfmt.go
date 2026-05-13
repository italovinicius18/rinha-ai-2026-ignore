// Package idxfmt defines the on-disk layout of the reference index built from
// references.json.gz, and provides streaming Writer + buffer Reader helpers.
//
// File layout (little-endian):
//
//	header  64 B   magic "RINH2026" | version u16 | format u16 | count u32 |
//	               dim u8 | reserved 3 B | crc32 u32 | reserved 40 B
//	body         depends on format:
//	             FormatFlat32: vectors (count × 14 × float32)
//	                           ‖ labels (ceil(count/8) bytes, 1=fraud)
//
// The CRC32 in the header covers the body only (everything after byte 64).
package idxfmt

import (
	"encoding/binary"
	"fmt"
)

const (
	HeaderSize = 64
	Dim        = 14
	Magic      = "RINH2026"
	Version    = uint16(1)

	FormatFlat32   = uint16(1)
	FormatInt8     = uint16(2)
	FormatIVF      = uint16(3) // IVF-int8: C centroids + int8 cluster vectors
	FormatIVF_F32  = uint16(4) // IVF-float32: C centroids + float32 cluster vectors
)

// Header is the parsed form of the on-disk header.
type Header struct {
	Version uint16
	Format  uint16
	Count   uint32
	Dim     uint8
	CRC32   uint32
}

// Marshal renders h into the on-disk byte layout. Reserved bytes are zeroed.
func (h Header) Marshal() [HeaderSize]byte {
	var b [HeaderSize]byte
	copy(b[0:8], Magic)
	binary.LittleEndian.PutUint16(b[8:10], h.Version)
	binary.LittleEndian.PutUint16(b[10:12], h.Format)
	binary.LittleEndian.PutUint32(b[12:16], h.Count)
	b[16] = h.Dim
	binary.LittleEndian.PutUint32(b[20:24], h.CRC32)
	return b
}

// ParseHeader extracts the header from the start of b.
func ParseHeader(b []byte) (Header, error) {
	if len(b) < HeaderSize {
		return Header{}, fmt.Errorf("idxfmt: buffer too short (%d < %d)", len(b), HeaderSize)
	}
	if string(b[0:8]) != Magic {
		return Header{}, fmt.Errorf("idxfmt: bad magic %q", string(b[0:8]))
	}
	h := Header{
		Version: binary.LittleEndian.Uint16(b[8:10]),
		Format:  binary.LittleEndian.Uint16(b[10:12]),
		Count:   binary.LittleEndian.Uint32(b[12:16]),
		Dim:     b[16],
		CRC32:   binary.LittleEndian.Uint32(b[20:24]),
	}
	if h.Version != Version {
		return h, fmt.Errorf("idxfmt: unsupported version %d (want %d)", h.Version, Version)
	}
	if h.Dim != Dim {
		return h, fmt.Errorf("idxfmt: unexpected dim %d (want %d)", h.Dim, Dim)
	}
	return h, nil
}
