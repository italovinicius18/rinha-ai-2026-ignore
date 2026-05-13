package brutei8

import (
	"os"
	"path/filepath"
	"testing"

	"rinha2026/solution/internal/idxfmt"
)

// buildMini writes a FormatInt8 index to a temp file and returns its bytes.
func buildMini(t testing.TB, vecs [][idxfmt.Dim]float64, labels []bool) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mini.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	w, err := idxfmt.NewInt8Writer(f)
	if err != nil {
		t.Fatalf("NewInt8Writer: %v", err)
	}
	for i, v := range vecs {
		if err := w.Add(v, labels[i]); err != nil {
			t.Fatalf("Add[%d]: %v", i, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	f.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return data
}

// TestSearchKNN_AllLegit: 10 legit entries, query at origin → 0 fraud.
func TestSearchKNN_AllLegit(t *testing.T) {
	vecs := make([][idxfmt.Dim]float64, 10)
	for i := range vecs {
		for j := range vecs[i] {
			vecs[i][j] = float64(i) * 0.01
		}
	}
	labels := make([]bool, 10) // all legit
	data := buildMini(t, vecs, labels)
	ix, err := Open(data)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var q [idxfmt.Dim]uint8
	if got := ix.SearchKNN(&q); got != 0 {
		t.Errorf("expected 0 fraud, got %d", got)
	}
}

// TestSearchKNN_AllFraud: 10 fraud entries, query near them → K=5 fraud.
func TestSearchKNN_AllFraud(t *testing.T) {
	vecs := make([][idxfmt.Dim]float64, 10)
	for i := range vecs {
		for j := range vecs[i] {
			vecs[i][j] = 0.5 + float64(i)*0.001
		}
	}
	labels := make([]bool, 10)
	for i := range labels {
		labels[i] = true
	}
	data := buildMini(t, vecs, labels)
	ix, err := Open(data)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var q [idxfmt.Dim]uint8
	for j := range q {
		q[j] = idxfmt.QuantizeFloat64(0.5)
	}
	if got := ix.SearchKNN(&q); got != K {
		t.Errorf("expected %d fraud, got %d", K, got)
	}
}

// TestSearchKNN_SentinelSeparation: null-tx entries (sentinel dims 5,6 = 255)
// form their own cluster; a query with sentinel dims should prefer them.
func TestSearchKNN_SentinelSeparation(t *testing.T) {
	vecs := make([][idxfmt.Dim]float64, 12)
	labels := make([]bool, 12)

	// Entries 0-5: fraud, sentinel at dims 5,6.
	for i := 0; i < 6; i++ {
		for j := range vecs[i] {
			vecs[i][j] = 0.3
		}
		vecs[i][5] = -1
		vecs[i][6] = -1
		labels[i] = true
	}
	// Entries 6-11: legit, valid dims 5,6.
	for i := 6; i < 12; i++ {
		for j := range vecs[i] {
			vecs[i][j] = 0.3
		}
		labels[i] = false
	}

	data := buildMini(t, vecs, labels)
	ix, err := Open(data)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Query with sentinel dims → should hit fraud cluster.
	var q [idxfmt.Dim]uint8
	for j := range q {
		q[j] = idxfmt.QuantizeFloat64(0.3)
	}
	q[5] = 255
	q[6] = 255

	if got := ix.SearchKNN(&q); got != K {
		t.Errorf("sentinel query: expected %d fraud, got %d", K, got)
	}
}

// TestQuantizeRoundTrip ensures QuantizeFloat64 maps sentinel and boundary values correctly.
func TestQuantizeRoundTrip(t *testing.T) {
	cases := []struct {
		in   float64
		want uint8
	}{
		{-1, 255},
		{-0.5, 255},
		{0.0, 0},
		{0.5, 127},
		{1.0, 254},
		{1.5, 254}, // clamped
	}
	for _, c := range cases {
		got := idxfmt.QuantizeFloat64(c.in)
		if got != c.want {
			t.Errorf("QuantizeFloat64(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func BenchmarkSearchKNN_100k(b *testing.B) {
	const N = 100_000
	vecs := make([][idxfmt.Dim]float64, N)
	labels := make([]bool, N)
	for i := range vecs {
		for j := range vecs[i] {
			vecs[i][j] = float64(i%1000) / 1000.0
		}
		labels[i] = i%3 == 0
	}
	data := buildMini(b, vecs, labels)
	ix, _ := Open(data)
	var q [idxfmt.Dim]uint8
	for j := range q {
		q[j] = 127
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		ix.SearchKNN(&q)
	}
}
