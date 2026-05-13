package ivf

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
	nprobe := 8
	b.Logf("index: %d vectors, %d centroids, nprobe=%d", ix.Count(), ix.NCentroids(), nprobe)

	var q [idxfmt.Dim]float32
	for j := range q {
		q[j] = 0.5
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		ix.SearchKNN(&q, nprobe)
	}
}
