package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"rinha2026/solution/internal/idxfmt"
	"rinha2026/solution/internal/ivf"
	"rinha2026/solution/internal/mmapfile"
	"rinha2026/solution/internal/vec"
)

type entry struct {
	Request          vec.Payload `json:"request"`
	ExpectedApproved bool        `json:"expected_approved"`
}
type testData struct {
	Entries []entry `json:"entries"`
}

func main() {
	idxPath := os.Getenv("IVF_INDEX")
	tdPath := os.Getenv("TEST_DATA")
	if idxPath == "" || tdPath == "" {
		log.Fatal("IVF_INDEX and TEST_DATA env vars required")
	}

	mf, err := mmapfile.Open(idxPath)
	if err != nil { log.Fatalf("mmap: %v", err) }
	ix, err := ivf.Open(mf.Data())
	if err != nil { log.Fatalf("open ivf: %v", err) }
	log.Printf("index: %d vectors, %d centroids", ix.Count(), ix.NCentroids())

	f, err := os.Open(tdPath)
	if err != nil { log.Fatalf("open test-data: %v", err) }
	defer f.Close()
	var rd interface{ Read([]byte) (int, error) } = f
	if len(tdPath) > 3 && tdPath[len(tdPath)-3:] == ".gz" {
		gz, err := gzip.NewReader(f)
		if err != nil { log.Fatalf("gzip: %v", err) }
		defer gz.Close()
		rd = gz
	}
	var td testData
	if err := json.NewDecoder(rd).Decode(&td); err != nil {
		log.Fatalf("decode: %v", err)
	}

	nprobe := 16
	dist := [6]int{}
	errDist := [6]int{}
	var total, errors int
	for i := range td.Entries {
		e := &td.Entries[i]
		v64, err := vec.Vectorize(&e.Request)
		if err != nil { continue }
		var q [idxfmt.Dim]float32
		for j := 0; j < idxfmt.Dim; j++ { q[j] = float32(v64[j]) }

		nFraud := ix.SearchKNN(&q, nprobe)
		approved := nFraud < 3
		dist[nFraud]++
		if approved != e.ExpectedApproved {
			errDist[nFraud]++
			errors++
		}
		total++
	}
	fmt.Printf("total=%d errors=%d\n", total, errors)
	fmt.Printf("nFraud distribution (all queries):   0=%d 1=%d 2=%d 3=%d 4=%d 5=%d\n",
		dist[0], dist[1], dist[2], dist[3], dist[4], dist[5])
	fmt.Printf("nFraud distribution (error queries): 0=%d 1=%d 2=%d 3=%d 4=%d 5=%d\n",
		errDist[0], errDist[1], errDist[2], errDist[3], errDist[4], errDist[5])
}
