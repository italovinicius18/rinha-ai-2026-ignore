// exp01-brute-float32 is the brute-force float32 KNN baseline. It is the
// correctness oracle for every other experiment: slow on purpose, but
// guaranteed to match the labels produced by exact k=5 Euclidean KNN over
// the published reference dataset.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"rinha2026/solution/internal/brutef32"
	"rinha2026/solution/internal/idxfmt"
	"rinha2026/solution/internal/mmapfile"
	"rinha2026/solution/internal/vec"
)

type response struct {
	Approved   bool    `json:"approved"`
	FraudScore float64 `json:"fraud_score"`
}

// Six possible fraud_score values (k=5 ⇒ nFraud/5 ∈ {0, 0.2, 0.4, 0.6, 0.8, 1.0}).
// Pre-rendered JSON bodies to avoid encoder allocations on the hot path.
var responseBodies = func() [6][]byte {
	var out [6][]byte
	for n := uint8(0); n <= 5; n++ {
		score := float64(n) / 5.0
		approved := score < 0.6
		b, err := json.Marshal(response{Approved: approved, FraudScore: score})
		if err != nil {
			log.Fatal(err)
		}
		out[n] = b
	}
	return out
}()

var payloadPool = sync.Pool{
	New: func() any { return new(vec.Payload) },
}

type server struct {
	ix    *brutef32.Index
	ready chan struct{}
}

func (s *server) handleReady(w http.ResponseWriter, _ *http.Request) {
	select {
	case <-s.ready:
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}
}

func (s *server) handleFraudScore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p := payloadPool.Get().(*vec.Payload)
	defer func() {
		*p = vec.Payload{}
		payloadPool.Put(p)
	}()

	if err := json.NewDecoder(r.Body).Decode(p); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	v64, err := vec.Vectorize(p)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var v32 [idxfmt.Dim]float32
	for i := 0; i < idxfmt.Dim; i++ {
		v32[i] = float32(v64[i])
	}
	nFraud := s.ix.SearchKNN(&v32)

	body := responseBodies[nFraud]
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func main() {
	addr := getenv("LISTEN_ADDR", ":8080")
	indexPath := getenv("INDEX_PATH", "/data/index.bin")

	srv := &server{ready: make(chan struct{})}

	mux := http.NewServeMux()
	mux.HandleFunc("/ready", srv.handleReady)
	mux.HandleFunc("/fraud-score", srv.handleFraudScore)

	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	httpSrv.SetKeepAlivesEnabled(true)

	go func() {
		log.Printf("opening index %s", indexPath)
		m, err := mmapfile.Open(indexPath)
		if err != nil {
			log.Fatalf("mmap index: %v", err)
		}
		ix, err := brutef32.Open(m.Data())
		if err != nil {
			log.Fatalf("open index: %v", err)
		}
		log.Printf("index ready: %d entries, %d bytes mapped", ix.Count(), m.Len())
		srv.ix = ix
		close(srv.ready)
	}()

	log.Printf("api listening on %s", addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// itoa avoids strconv import to keep dependencies tight (lengths ≤ 4 chars).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [4]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
