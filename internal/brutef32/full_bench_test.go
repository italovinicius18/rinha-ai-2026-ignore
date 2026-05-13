package brutef32

import (
	"math/rand"
	"os"
	"testing"

	"rinha2026/solution/internal/idxfmt"
	"rinha2026/solution/internal/mmapfile"
)

// BenchmarkSearchKNN_Full3M times brute-force KNN against the real 3M-vector
// index. Skipped unless RINHA_INDEX points to a built FLAT32 file.
//
// Run with: docker compose -or- ad-hoc:
//
//	docker run --rm -v $PWD:/src -v /tmp/rinha-out:/out -w /src \
//	   golang:1.23-alpine sh -c \
//	   'go test -run=^$ -bench=BenchmarkSearchKNN_Full3M -benchtime=20x \
//	         -count=1 ./internal/brutef32 -- -index=/out/full.bin'
//
// (or via the env var RINHA_INDEX=/out/full.bin)
func BenchmarkSearchKNN_Full3M(b *testing.B) {
	path := os.Getenv("RINHA_INDEX")
	if path == "" {
		b.Skip("RINHA_INDEX env var not set; skipping full-3M benchmark")
	}
	m, err := mmapfile.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer m.Close()
	ix, err := Open(m.Data())
	if err != nil {
		b.Fatal(err)
	}
	b.Logf("index: %d entries, %d bytes mmap'd", ix.Count(), m.Len())

	rng := rand.New(rand.NewSource(1))
	var q [idxfmt.Dim]float32
	for j := 0; j < idxfmt.Dim; j++ {
		q[j] = rng.Float32()
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ix.SearchKNN(&q)
	}
	if b.N > 0 {
		ms := float64(b.Elapsed().Microseconds()) / float64(b.N) / 1000.0
		b.ReportMetric(ms, "ms/query")
	}
}
