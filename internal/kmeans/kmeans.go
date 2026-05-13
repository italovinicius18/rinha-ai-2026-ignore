// Package kmeans provides a parallel k-means implementation over float32
// row-major vectors for offline index building (no CGO, no SIMD).
//
// Initialization: k-means++ with O(n×k) incremental distance updates,
// parallelized per-centroid across available CPUs.
// Assignment: parallel over available CPUs.
// Update: serial accumulation into float64 sums.
package kmeans

import (
	"math"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

// ProgressFn is an optional callback called after each iteration.
// It receives (iteration, changed) where changed is assignments that flipped.
type ProgressFn func(iter int, changed int64)

// Fit runs k-means++ on vecs (n×dim row-major float32, where n = len(vecs)/dim).
// Returns (centroids, assign): centroids is k×dim float32; assign[i] is the
// cluster index for vec i.
func Fit(vecs []float32, dim, k, maxIter int, seed int64) (centroids []float32, assign []int32) {
	return FitWithProgress(vecs, dim, k, maxIter, seed, nil)
}

// FitWithProgress is like Fit but calls progress after each iteration.
func FitWithProgress(vecs []float32, dim, k, maxIter int, seed int64, progress ProgressFn) (centroids []float32, assign []int32) {
	n := len(vecs) / dim
	rng := rand.New(rand.NewSource(seed))

	centroids = initPlusPlus(vecs, dim, n, k, rng)
	assign = make([]int32, n)

	sums := make([]float64, k*dim)
	counts := make([]int64, k)

	for iter := 0; iter < maxIter; iter++ {
		changed := parallelAssign(vecs, centroids, assign, dim, n, k)

		// Update: accumulate per-cluster sums.
		for i := range sums {
			sums[i] = 0
		}
		for i := range counts {
			counts[i] = 0
		}
		for i := 0; i < n; i++ {
			c := int(assign[i])
			counts[c]++
			base := i * dim
			for d := 0; d < dim; d++ {
				sums[c*dim+d] += float64(vecs[base+d])
			}
		}
		for c := 0; c < k; c++ {
			if counts[c] == 0 {
				continue
			}
			inv := 1.0 / float64(counts[c])
			for d := 0; d < dim; d++ {
				centroids[c*dim+d] = float32(sums[c*dim+d] * inv)
			}
		}

		if progress != nil {
			progress(iter, changed)
		}
		if changed == 0 {
			break
		}
	}
	return centroids, assign
}

// parallelAssign assigns each vector to its nearest centroid in parallel.
// Returns the number of changed assignments.
func parallelAssign(vecs, centroids []float32, assign []int32, dim, n, k int) int64 {
	nW := runtime.GOMAXPROCS(0)
	if nW > n {
		nW = n
	}
	chunk := (n + nW - 1) / nW

	var total int64
	var wg sync.WaitGroup
	for w := 0; w < nW; w++ {
		start := w * chunk
		end := start + chunk
		if end > n {
			end = n
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			var local int64
			for i := start; i < end; i++ {
				v := vecs[i*dim : (i+1)*dim]
				best := 0
				bestD := float32(math.MaxFloat32)
				for c := 0; c < k; c++ {
					d := sqDist(v, centroids[c*dim:(c+1)*dim], dim)
					if d < bestD {
						best, bestD = c, d
					}
				}
				if assign[i] != int32(best) {
					assign[i] = int32(best)
					local++
				}
			}
			atomic.AddInt64(&total, local)
		}(start, end)
	}
	wg.Wait()
	return total
}

// initPlusPlus seeds k centroids using k-means++ D² sampling.
// Complexity: O(n×k×dim) via incremental distance maintenance.
// The inner per-centroid update is parallelized.
func initPlusPlus(vecs []float32, dim, n, k int, rng *rand.Rand) []float32 {
	centroids := make([]float32, k*dim)
	dists := make([]float32, n)

	// Must be MaxFloat32 so atomic-min works on the first update.
	for i := range dists {
		dists[i] = math.MaxFloat32
	}

	// Centroid 0: uniform random.
	first := rng.Intn(n)
	copy(centroids[:dim], vecs[first*dim:(first+1)*dim])

	// Update dists: dists[i] = dist(i, centroid0).
	parallelUpdateDists(vecs, centroids[:dim], dists, dim, n)

	for ci := 1; ci < k; ci++ {
		// Sample next centroid proportional to D².
		var totalD float64
		for _, d := range dists {
			totalD += float64(d)
		}
		target := rng.Float64() * totalD
		var acc float64
		chosen := n - 1
		for i, d := range dists {
			acc += float64(d)
			if acc >= target {
				chosen = i
				break
			}
		}
		copy(centroids[ci*dim:(ci+1)*dim], vecs[chosen*dim:(chosen+1)*dim])

		// Incrementally update dists: dists[i] = min(dists[i], dist(i, new_centroid)).
		parallelUpdateDists(vecs, centroids[ci*dim:(ci+1)*dim], dists, dim, n)
	}
	return centroids
}

// parallelUpdateDists sets dists[i] = min(dists[i], dist(vecs[i], centroid)).
func parallelUpdateDists(vecs []float32, centroid []float32, dists []float32, dim, n int) {
	nW := runtime.GOMAXPROCS(0)
	if nW > n {
		nW = n
	}
	chunk := (n + nW - 1) / nW
	var wg sync.WaitGroup
	for w := 0; w < nW; w++ {
		start := w * chunk
		end := start + chunk
		if end > n {
			end = n
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for i := start; i < end; i++ {
				d := sqDist(vecs[i*dim:(i+1)*dim], centroid, dim)
				// Atomic min on float32 (stored as uint32 in dists).
				// Safe because we never set dists[i] to NaN or ±Inf.
				for {
					old := *(*uint32)(unsafe.Pointer(&dists[i]))
					if d >= math.Float32frombits(old) {
						break
					}
					if atomic.CompareAndSwapUint32((*uint32)(unsafe.Pointer(&dists[i])), old, math.Float32bits(d)) {
						break
					}
				}
			}
		}(start, end)
	}
	wg.Wait()
}

func sqDist(a, b []float32, dim int) float32 {
	var s float32
	for i := 0; i < dim; i++ {
		d := a[i] - b[i]
		s += d * d
	}
	return s
}
