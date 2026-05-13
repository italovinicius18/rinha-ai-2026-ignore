// build-ivf builds a FormatIVF (int8) or FormatIVF_F32 (float32) index from an
// existing FormatFlat32 index.
//
// Usage:
//
//	build-ivf --in=full.bin --out=ivf.bin [--centroids=256] [--iters=30] [--dtype=int8|float32]
//
// Pipeline:
//  1. Mmap the flat32 index and load all float32 vectors.
//  2. Run parallel k-means++ to get C centroids.
//  3. Assign every vector to its nearest centroid.
//  4. Write the IVF index via idxfmt.IVFWriter (int8) or IVFWriterF32 (float32).
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"time"

	"rinha2026/solution/internal/idxfmt"
	"rinha2026/solution/internal/kmeans"
	"rinha2026/solution/internal/mmapfile"
)

func main() {
	in := flag.String("in", "", "input FormatFlat32 index file (required)")
	out := flag.String("out", "", "output IVF index file (required)")
	nCentroids := flag.Int("centroids", 256, "number of IVF centroids (k-means clusters)")
	maxIter := flag.Int("iters", 30, "maximum k-means iterations")
	seed := flag.Int64("seed", 42, "random seed for k-means++")
	dtype := flag.String("dtype", "int8", "cluster vector dtype: int8 or float32")
	flag.Parse()

	if *in == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}

	useF32 := *dtype == "float32"
	log.Printf("build-ivf: GOMAXPROCS=%d dtype=%s", runtime.GOMAXPROCS(0), *dtype)
	start := time.Now()

	// ── 1. Load float32 vectors from FLAT32 index ──────────────────────────
	mf, err := mmapfile.Open(*in)
	if err != nil {
		log.Fatalf("mmap %s: %v", *in, err)
	}
	defer mf.Close()

	data := mf.Data()
	h, err := idxfmt.ParseHeader(data)
	if err != nil {
		log.Fatalf("parse header: %v", err)
	}
	if h.Format != idxfmt.FormatFlat32 {
		log.Fatalf("input is not FormatFlat32 (got format %d)", h.Format)
	}
	n := int(h.Count)
	log.Printf("loaded %d vectors from %s", n, *in)

	vecBytes := data[idxfmt.HeaderSize : idxfmt.HeaderSize+n*idxfmt.Dim*4]
	vecs32 := make([]float32, n*idxfmt.Dim)
	for i := range vecs32 {
		vecs32[i] = math.Float32frombits(binary.LittleEndian.Uint32(vecBytes[i*4:]))
	}

	lblOff := idxfmt.HeaderSize + n*idxfmt.Dim*4
	lblBytes := data[lblOff : lblOff+(n+7)/8]

	log.Printf("parsed vectors in %s", time.Since(start).Round(time.Millisecond))

	// ── 2. K-means++ clustering ────────────────────────────────────────────
	kmStart := time.Now()
	log.Printf("running k-means++: C=%d, maxIter=%d, GOMAXPROCS=%d ...", *nCentroids, *maxIter, runtime.GOMAXPROCS(0))
	centroids, assign := kmeans.FitWithProgress(vecs32, idxfmt.Dim, *nCentroids, *maxIter, *seed,
		func(iter int, changed int64) {
			log.Printf("  iter %2d: %d reassigned (%s)", iter+1, changed, time.Since(kmStart).Round(time.Millisecond))
		})
	log.Printf("k-means done in %s", time.Since(kmStart).Round(time.Millisecond))

	// ── 3. Count cluster sizes ─────────────────────────────────────────────
	sizes := make([]uint32, *nCentroids)
	for _, c := range assign {
		sizes[c]++
	}
	minSz, maxSz := sizes[0], sizes[0]
	for _, s := range sizes {
		if s < minSz {
			minSz = s
		}
		if s > maxSz {
			maxSz = s
		}
	}
	log.Printf("cluster sizes: min=%d avg=%d max=%d", minSz, uint32(n / *nCentroids), maxSz)

	// ── 4. Build per-cluster vectors and labels ────────────────────────────
	type clusterData struct {
		i8vecs  []byte
		f32vecs []float32
		labels  []bool
	}
	clusters := make([]clusterData, *nCentroids)
	for c := range clusters {
		sz := int(sizes[c])
		clusters[c].labels = make([]bool, sz)
		if useF32 {
			clusters[c].f32vecs = make([]float32, sz*idxfmt.Dim)
		} else {
			clusters[c].i8vecs = make([]byte, sz*idxfmt.Dim)
		}
	}

	pos := make([]int, *nCentroids)
	for i := 0; i < n; i++ {
		c := int(assign[i])
		p := pos[c]
		if useF32 {
			copy(clusters[c].f32vecs[p*idxfmt.Dim:], vecs32[i*idxfmt.Dim:(i+1)*idxfmt.Dim])
		} else {
			for d := 0; d < idxfmt.Dim; d++ {
				clusters[c].i8vecs[p*idxfmt.Dim+d] = idxfmt.QuantizeFloat64(float64(vecs32[i*idxfmt.Dim+d]))
			}
		}
		clusters[c].labels[p] = lblBytes[i/8]&(1<<(i%8)) != 0
		pos[c]++
	}
	log.Printf("clustered vectors in %s", time.Since(start).Round(time.Millisecond))

	// ── 5. Write IVF index ─────────────────────────────────────────────────
	outF, err := os.Create(*out)
	if err != nil {
		log.Fatalf("create %s: %v", *out, err)
	}
	var w *idxfmt.IVFWriter
	if useF32 {
		w, err = idxfmt.NewIVFWriterF32(outF, centroids, sizes)
	} else {
		w, err = idxfmt.NewIVFWriter(outF, centroids, sizes)
	}
	if err != nil {
		log.Fatalf("NewIVFWriter: %v", err)
	}
	for c := 0; c < *nCentroids; c++ {
		if useF32 {
			if err := w.AddClusterF32(clusters[c].f32vecs, clusters[c].labels); err != nil {
				log.Fatalf("AddClusterF32 %d: %v", c, err)
			}
		} else {
			if err := w.AddCluster(clusters[c].i8vecs, clusters[c].labels); err != nil {
				log.Fatalf("AddCluster %d: %v", c, err)
			}
		}
	}
	if err := w.Close(); err != nil {
		log.Fatalf("Close: %v", err)
	}
	if err := outF.Close(); err != nil {
		log.Fatalf("close file: %v", err)
	}

	fi, _ := os.Stat(*out)
	log.Printf("done · %d vectors · C=%d · dtype=%s · output=%s (%dB) · total %s",
		n, *nCentroids, *dtype, *out, fi.Size(), time.Since(start).Round(time.Millisecond))
	fmt.Printf("ivf_index_bytes=%d\n", fi.Size())
}
