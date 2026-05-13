// exp14-profiling: identical to exp09 (IVF-F32 C=512 nprobe=8) but with per-phase
// timing instrumentation. Every 1000th request logs a sample breakdown:
// decode / vectorize+cast / search / total handler time.
// This tells us where the ~160ms overhead actually lives.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"rinha2026/solution/internal/idxfmt"
	"rinha2026/solution/internal/ivf"
	"rinha2026/solution/internal/mmapfile"
	"rinha2026/solution/internal/vec"
)

const defaultNprobe = 16
const sampleEvery = 1000

type response struct {
	Approved   bool    `json:"approved"`
	FraudScore float64 `json:"fraud_score"`
}

var responseBodies = func() [6][]byte {
	var out [6][]byte
	for n := uint8(0); n <= 5; n++ {
		score := float64(n) / 5.0
		b, err := json.Marshal(response{Approved: score < 0.6, FraudScore: score})
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
	ix      *ivf.Index
	nprobe  int
	ready   chan struct{}
	counter atomic.Int64
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

	t0 := time.Now()
	n := s.counter.Add(1)
	doSample := n%sampleEvery == 0

	p := payloadPool.Get().(*vec.Payload)
	defer func() {
		*p = vec.Payload{}
		payloadPool.Put(p)
	}()

	if err := json.NewDecoder(r.Body).Decode(p); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	tDecoded := time.Now()

	v64, err := vec.Vectorize(p)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var q [idxfmt.Dim]float32
	for i := 0; i < idxfmt.Dim; i++ {
		q[i] = float32(v64[i])
	}
	tVec := time.Now()

	nFraud := s.ix.SearchKNN(&q, s.nprobe)
	tSearch := time.Now()

	body := responseBodies[nFraud]
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)

	if doSample {
		total := time.Since(t0)
		decodeT := tDecoded.Sub(t0)
		vecT := tVec.Sub(tDecoded)
		searchT := tSearch.Sub(tVec)
		log.Printf("sample#%d decode=%s vec=%s search=%s total=%s",
			n, decodeT.Round(time.Microsecond),
			vecT.Round(time.Microsecond),
			searchT.Round(time.Microsecond),
			total.Round(time.Microsecond))
	}
}

func main() {
	addr := getenv("LISTEN_ADDR", ":8080")
	indexPath := getenv("INDEX_PATH", "/data/index.bin")
	nprobe := defaultNprobe
	if s := os.Getenv("NPROBE"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			nprobe = n
		}
	}

	srv := &server{nprobe: nprobe, ready: make(chan struct{})}

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
		log.Printf("opening IVF-F32 index %s (nprobe=%d)", indexPath, nprobe)
		m, err := mmapfile.Open(indexPath)
		if err != nil {
			log.Fatalf("mmap index: %v", err)
		}
		ix, err := ivf.Open(m.Data())
		if err != nil {
			log.Fatalf("open index: %v", err)
		}
		log.Printf("index ready: %d vectors, %d centroids, %d bytes mapped",
			ix.Count(), ix.NCentroids(), m.Len())
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
