// Package search provides brute-force and approximate nearest-neighbor
// search over labeled reference vectors for fraud detection.
package search

import (
	"container/heap"
	"fmt"
	"os"
	"slices"
	"strconv"

	"github.com/leandro-lugaresi/rinha-2026-go/internal/reference"
)

// ivfDimCount is the fixed dimensionality of the vector space.
const ivfDimCount = 14

// defaultProbeCount is the number of IVF partitions probed when no explicit
// probe count is given and IVF_PROBE_COUNT is not set.
const defaultProbeCount = 32

// IVFSearcher performs approximate nearest-neighbor search using an
// Inverted File Index.
type IVFSearcher struct {
	index      *reference.IVFIndex
	probeCount int
}

// NewIVFSearcher creates a searcher that probes the specified number of
// nearest partitions during search. If probeCount ≤ 0, the default from
// the IVF_PROBE_COUNT environment variable is used (falling back to 32).
func NewIVFSearcher(idx *reference.IVFIndex, probeCount int) *IVFSearcher {
	if probeCount <= 0 {
		probeCount = probeCountFromEnv()
	}
	return &IVFSearcher{index: idx, probeCount: probeCount}
}

// probeCountFromEnv reads IVF_PROBE_COUNT from the environment, defaulting
// to defaultProbeCount if unset or unparseable.
func probeCountFromEnv() int {
	s := os.Getenv("IVF_PROBE_COUNT")
	if s == "" {
		return defaultProbeCount
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultProbeCount
	}
	return n
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

// centroidDist pairs a partition index with its distance from the query.
type centroidDist struct {
	idx  int
	dist float64
}

// neighbor holds a candidate reference with its distance from the query.
type neighbor struct {
	ref  reference.Reference
	dist uint64 // squared integer distance over all 14 uint8 dims
}

// maxHeap implements the heap.Interface to keep the K smallest distances.
// The top of the heap is the largest distance among the K (a max-heap).
type maxHeap struct {
	items []neighbor
}

func (h maxHeap) Len() int           { return len(h.items) }
func (h maxHeap) Less(i, j int) bool { return h.items[i].dist > h.items[j].dist }
func (h maxHeap) Swap(i, j int)      { h.items[i], h.items[j] = h.items[j], h.items[i] }

func (h *maxHeap) Push(x any) {
	h.items = append(h.items, x.(neighbor))
}

func (h *maxHeap) Pop() any {
	old := h.items
	n := len(old)
	x := old[n-1]
	h.items = old[:n-1]
	return x
}

// Search finds the k nearest neighbors using the IVF index.
//
// Algorithm:
//  1. Quantize the query to uint8 (for integer distance against stored vectors).
//  2. Compute squared Euclidean distance from query (float64) to all centroids (float32).
//  3. Select probeCount nearest centroids.
//  4. Scan ALL vectors in those partitions; for each, compute squared Euclidean
//     distance over all 14 uint8 dimensions.
//  5. Keep top-K using a max-heap.
//  6. If fewer than K candidates after probing probeCount partitions, expand to
//     subsequent nearest partitions until K candidates are collected or the index
//     is exhausted.
//  7. Return up to K nearest references sorted by distance ascending.
func (s *IVFSearcher) Search(query [14]float64, k int) ([]reference.Reference, error) {
	if k <= 0 {
		return nil, fmt.Errorf("k must be positive, got %d", k)
	}
	totalVectors := int(s.index.VectorCount())
	if totalVectors == 0 {
		return nil, fmt.Errorf("index is empty")
	}

	// 1. Quantize query for integer distance comparison against stored uint8 vectors.
	qUint8 := reference.QuantizeVector(query)

	// 2. Compute squared Euclidean distance to every centroid (float64→float32).
	nParts := int(s.index.PartitionCount())
	byDist := make([]centroidDist, nParts)
	for p := 0; p < nParts; p++ {
		c := s.index.Centroid(p)
		var d float64
		for j := 0; j < ivfDimCount; j++ {
			diff := query[j] - float64(c[j])
			d += diff * diff
		}
		byDist[p] = centroidDist{idx: p, dist: d}
	}

	slices.SortFunc(byDist, func(a, b centroidDist) int {
		if a.dist < b.dist {
			return -1
		}
		if a.dist > b.dist {
			return 1
		}
		return 0
	})

	// 3–5. Scan partitions, keep top-K in a max-heap.
	h := &maxHeap{}
	heap.Init(h)

	for probe := 0; probe < nParts; probe++ {
		p := byDist[probe]

		// PartitionOffset is a byte offset; divide by ivfDimCount to get
		// the 0-based vector index.
		pid := p.idx
		byteOff := s.index.PartitionOffset(pid)
		vecStart := uint32(byteOff / ivfDimCount)
		vecCount := s.index.PartitionVectorCount(pid)

		for i := uint32(0); i < vecCount; i++ {
			vi := vecStart + i
			vec := s.index.VectorAt(vi)

			var d uint64
			for j := 0; j < ivfDimCount; j++ {
				diff := int64(qUint8[j]) - int64(vec[j])
				d += uint64(diff * diff)
			}

			ref := reference.Reference{
				Vector: dequantizeVector(vec),
				Label:  labelStr(s.index.LabelAt(vi)),
			}

			if h.Len() < k {
				heap.Push(h, neighbor{ref: ref, dist: d})
			} else if d < h.items[0].dist {
				heap.Pop(h)
				heap.Push(h, neighbor{ref: ref, dist: d})
			}
		}

		if probe+1 >= s.probeCount && h.Len() >= k {
			break
		}
	}

	if h.Len() == 0 {
		return nil, fmt.Errorf("no candidates found in index")
	}

	slices.SortFunc(h.items, func(a, b neighbor) int {
		if a.dist < b.dist {
			return -1
		}
		if a.dist > b.dist {
			return 1
		}
		return 0
	})

	resultLen := h.Len()
	result := make([]reference.Reference, resultLen)
	for i, n := range h.items {
		result[i] = n.ref
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// dequantizeVector converts a stored uint8 vector back to float64.
// Sentinel handling: 255 on dimensions 5 and 6 maps to -1 (no history marker);
// 255 on other dimensions simply means 1.0.
func dequantizeVector(v [14]uint8) [14]float64 {
	var out [14]float64
	for i := 0; i < ivfDimCount; i++ {
		if v[i] == 255 && (i == 5 || i == 6) {
			out[i] = -1.0
		} else {
			out[i] = float64(v[i]) / 255.0
		}
	}
	return out
}

// labelStr maps the stored uint8 label (0=legit, 1=fraud) to a string.
func labelStr(l uint8) string {
	if l == 1 {
		return "fraud"
	}
	return "legit"
}


