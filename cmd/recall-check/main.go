// recall-check walks every entry in test-data.json, runs brute-force KNN
// against the mmap'd FLAT32 index, and compares the result to expected_approved.
// Use this to validate that a new index variant hasn't degraded recall before
// spending 2 minutes on a full k6 run.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"rinha2026/solution/internal/brutef32"
	"rinha2026/solution/internal/idxfmt"
	"rinha2026/solution/internal/mmapfile"
	"rinha2026/solution/internal/vec"
)

// testData mirrors the top-level structure of test-data.json.
type testData struct {
	Entries []entry `json:"entries"`
}

type entry struct {
	Request         vec.Payload `json:"request"`
	ExpectedApproved bool       `json:"expected_approved"`
}

func main() {
	indexPath := flag.String("index", "", "path to FLAT32 index file (required)")
	testPath := flag.String("testdata", "", "path to test-data.json (required)")
	flag.Parse()

	if *indexPath == "" || *testPath == "" {
		flag.Usage()
		os.Exit(2)
	}

	// Load index.
	m, err := mmapfile.Open(*indexPath)
	if err != nil {
		log.Fatalf("mmap %s: %v", *indexPath, err)
	}
	defer m.Close()

	ix, err := brutef32.Open(m.Data())
	if err != nil {
		log.Fatalf("open index: %v", err)
	}
	log.Printf("index: %d entries", ix.Count())

	// Load test data.
	f, err := os.Open(*testPath)
	if err != nil {
		log.Fatalf("open %s: %v", *testPath, err)
	}
	defer f.Close()

	var td testData
	if err := json.NewDecoder(f).Decode(&td); err != nil {
		log.Fatalf("decode test-data: %v", err)
	}
	log.Printf("test entries: %d", len(td.Entries))

	start := time.Now()
	var tp, tn, fp, fn, errs int

	for i := range td.Entries {
		e := &td.Entries[i]
		v64, err := vec.Vectorize(&e.Request)
		if err != nil {
			errs++
			continue
		}
		var v32 [idxfmt.Dim]float32
		for j := 0; j < idxfmt.Dim; j++ {
			v32[j] = float32(v64[j])
		}
		nFraud := ix.SearchKNN(&v32)
		approved := nFraud < 3 // fraud_score = nFraud/5 < 0.6

		switch {
		case approved && e.ExpectedApproved:
			tn++
		case !approved && !e.ExpectedApproved:
			tp++
		case approved && !e.ExpectedApproved:
			fn++
		default:
			fp++
		}
	}

	elapsed := time.Since(start)
	total := tp + tn + fp + fn + errs
	correct := tp + tn
	failRate := float64(fp+fn+errs) / float64(total) * 100

	fmt.Printf("\n--- recall-check results ---\n")
	fmt.Printf("total:        %d\n", total)
	fmt.Printf("TP (fraud detected):    %d\n", tp)
	fmt.Printf("TN (legit approved):    %d\n", tn)
	fmt.Printf("FP (legit blocked):     %d\n", fp)
	fmt.Printf("FN (fraud approved):    %d\n", fn)
	fmt.Printf("errors (bad payload):   %d\n", errs)
	fmt.Printf("accuracy:     %.4f%%\n", float64(correct)/float64(total)*100)
	fmt.Printf("failure_rate: %.4f%%\n", failRate)
	fmt.Printf("elapsed:      %s  (%.1f ms/query)\n", elapsed.Round(time.Millisecond), float64(elapsed.Milliseconds())/float64(total))
}
