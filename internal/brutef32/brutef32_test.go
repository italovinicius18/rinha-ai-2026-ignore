package brutef32

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"rinha2026/solution/internal/idxfmt"
)

// writeIndex builds a small in-memory FLAT32 index from entries and returns
// the bytes.
func writeIndex(t *testing.T, entries []entry) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "idx.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w, err := idxfmt.NewFlatWriter(f)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if err := w.Add(e.v, e.fraud); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type entry struct {
	v     [idxfmt.Dim]float64
	fraud bool
}

func TestSearch_SelfDistanceZero(t *testing.T) {
	entries := []entry{
		{[idxfmt.Dim]float64{0.01, 0.0833, 0.05, 0.8261, 0.1667, -1, -1, 0.0432, 0.25, 0, 1, 0, 0.2, 0.0416}, false},
		{[idxfmt.Dim]float64{0.5796, 0.9167, 1.0, 0.0435, 0.0, 0.0056, 0.4394, 0.4598, 0.4, 1, 0, 1, 0.85, 0.0032}, true},
		{[idxfmt.Dim]float64{0.0035, 0.1667, 0.05, 0.4348, 0.6667, 0.1278, 0.0008, 0.017, 0.1, 0, 1, 0, 0.2, 0.02}, false},
		{[idxfmt.Dim]float64{0.9708, 1.0, 1.0, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 1, 0, 1, 0.9, 0.5}, true},
		{[idxfmt.Dim]float64{0.4082, 1.0, 1.0, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 1, 0, 1, 0.9, 0.5}, true},
	}
	data := writeIndex(t, entries)
	ix, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}

	// Query is the exact vector of entry 1 (fraud).
	var q [idxfmt.Dim]float32
	for j := 0; j < idxfmt.Dim; j++ {
		q[j] = float32(entries[1].v[j])
	}
	got := ix.SearchKNN(&q)
	// With only 5 entries and K=5, the search returns every label. Sample
	// has 3 frauds (entries 1, 3, 4).
	if got != 3 {
		t.Errorf("want 3 frauds among top-5, got %d", got)
	}
}

func TestSearch_LegitClusterReturnsAllLegit(t *testing.T) {
	// 6 entries: 5 close-by legits at origin + 1 far-away fraud.
	zero := [idxfmt.Dim]float64{}
	entries := []entry{
		{zero, false},
		{[idxfmt.Dim]float64{0.01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, false},
		{[idxfmt.Dim]float64{0, 0.01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, false},
		{[idxfmt.Dim]float64{0, 0, 0.01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, false},
		{[idxfmt.Dim]float64{0, 0, 0, 0.01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, false},
		{[idxfmt.Dim]float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, true},
	}
	data := writeIndex(t, entries)
	ix, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}

	var q [idxfmt.Dim]float32 // all zeros — query near the cluster
	got := ix.SearchKNN(&q)
	if got != 0 {
		t.Errorf("want 0 frauds in top-5 near the legit cluster, got %d", got)
	}
}

func TestSearch_FraudClusterReturnsAllFraud(t *testing.T) {
	// 5 close-by frauds + 1 far legit.
	entries := []entry{
		{[idxfmt.Dim]float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, true},
		{[idxfmt.Dim]float64{0.99, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, true},
		{[idxfmt.Dim]float64{1, 0.99, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, true},
		{[idxfmt.Dim]float64{1, 1, 0.99, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, true},
		{[idxfmt.Dim]float64{1, 1, 1, 0.99, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, true},
		{[idxfmt.Dim]float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, false},
	}
	data := writeIndex(t, entries)
	ix, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}

	q := [idxfmt.Dim]float32{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	got := ix.SearchKNN(&q)
	if got != 5 {
		t.Errorf("want 5 frauds in top-5 near the fraud cluster, got %d", got)
	}
}

func TestSearch_MixedNeighborhood(t *testing.T) {
	// 3 close frauds + 2 close legits + 1 far legit → expect 3 frauds.
	entries := []entry{
		{[idxfmt.Dim]float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, true},
		{[idxfmt.Dim]float64{0.01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, true},
		{[idxfmt.Dim]float64{0, 0.01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, true},
		{[idxfmt.Dim]float64{0.02, 0.01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, false},
		{[idxfmt.Dim]float64{0.01, 0.02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, false},
		{[idxfmt.Dim]float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, false},
	}
	data := writeIndex(t, entries)
	ix, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	q := [idxfmt.Dim]float32{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	got := ix.SearchKNN(&q)
	if got != 3 {
		t.Errorf("mixed neighborhood: want 3 frauds, got %d", got)
	}
}

func TestOpen_RejectsWrongFormat(t *testing.T) {
	// Build a 64-byte header with format=2 (int8) and no body. Open must reject.
	h := idxfmt.Header{
		Version: idxfmt.Version,
		Format:  idxfmt.FormatInt8,
		Count:   0,
		Dim:     idxfmt.Dim,
	}
	hb := h.Marshal()
	if _, err := Open(hb[:]); err == nil {
		t.Error("expected error for non-FLAT32 format")
	}
}

// Benchmark on a randomly-generated 200k-entry index. Caller can override
// the size with BENCH_N=<n>.
func BenchmarkSearchKNN(b *testing.B) {
	n := 200_000
	if v := os.Getenv("BENCH_N"); v != "" {
		var x int
		_, _ = parseN(v, &x)
		if x > 0 {
			n = x
		}
	}
	path := filepath.Join(b.TempDir(), "bench.bin")
	f, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	w, _ := idxfmt.NewFlatWriter(f)
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < n; i++ {
		var v [idxfmt.Dim]float64
		for j := 0; j < idxfmt.Dim; j++ {
			v[j] = rng.Float64()
		}
		_ = w.Add(v, i%3 == 0)
	}
	_ = w.Close()
	_ = f.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	ix, err := Open(data)
	if err != nil {
		b.Fatal(err)
	}

	var q [idxfmt.Dim]float32
	for j := 0; j < idxfmt.Dim; j++ {
		q[j] = rng.Float32()
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ix.SearchKNN(&q)
	}
	b.ReportMetric(float64(n)*float64(b.N)/b.Elapsed().Seconds(), "vec/s")
}

func parseN(s string, out *int) (int, error) {
	var x int
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, nil
		}
		x = x*10 + int(r-'0')
	}
	*out = x
	return x, nil
}
