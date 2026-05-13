package brutei8

import (
	"os"
	"testing"

	"rinha2026/solution/internal/idxfmt"
	"rinha2026/solution/internal/mmapfile"
)

func BenchmarkSearchKNN_Full3M(b *testing.B) {
	path := os.Getenv("RINHA_INDEX")
	if path == "" {
		b.Skip("RINHA_INDEX not set")
	}
	m, err := mmapfile.Open(path)
	if err != nil {
		b.Fatalf("mmap: %v", err)
	}
	defer m.Close()
	ix, err := Open(m.Data())
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	b.Logf("index: %d entries", ix.Count())

	var q [idxfmt.Dim]uint8
	for j := range q {
		q[j] = 127
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		ix.SearchKNN(&q)
	}
}
