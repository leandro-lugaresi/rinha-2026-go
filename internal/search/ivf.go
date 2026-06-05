// Package search provides brute-force and approximate nearest-neighbor
// search over labeled reference vectors for fraud detection.
package search

import (
	"fmt"
	"os"
	"strconv"

	"github.com/leandro-lugaresi/rinha-2026-go/internal/reference"
)

// ivfDimCount is the fixed dimensionality of the vector space.
const ivfDimCount = 14

const fraudLabel = "fraud"

// defaultProbeCount is the number of IVF partitions probed when no explicit
// probe count is given and IVF_PROBE_COUNT is not set.
const defaultProbeCount = 4

// IVFSearcher performs approximate nearest-neighbor search using an
// Inverted File Index.
type IVFSearcher struct {
	index      *reference.IVFIndex
	probeCount int
}

// NewIVFSearcher creates a searcher that probes the specified number of
// nearest partitions during search. If probeCount ≤ 0, the default from
// the IVF_PROBE_COUNT environment variable is used (falling back to 8).
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

// SearchLabels finds the k nearest neighbors and returns only their labels.
func (s *IVFSearcher) SearchLabels(query [14]float64, k int) ([]string, error) {
	if k <= 0 {
		return nil, fmt.Errorf("k must be positive, got %d", k)
	}
	totalVectors := int(s.index.VectorCount())
	if totalVectors == 0 {
		return nil, fmt.Errorf("index is empty")
	}

	qUint8 := reference.QuantizeVector(query)
	nParts := int(s.index.PartitionCount())

	type nc struct {
		idx  int
		dist float64
	}
	nearest := make([]nc, 0, s.probeCount)
	for p := 0; p < nParts; p++ {
		c := s.index.Centroid(p)
		d := dist14Centroid(query, c)
		if len(nearest) < s.probeCount {
			nearest = append(nearest, nc{idx: p, dist: d})
			i := len(nearest) - 1
			for i > 0 {
				parent := (i - 1) / 2
				if nearest[parent].dist >= nearest[i].dist {
					break
				}
				nearest[parent], nearest[i] = nearest[i], nearest[parent]
				i = parent
			}
		} else if d < nearest[0].dist {
			nearest[0] = nc{idx: p, dist: d}
			i := 0
			for {
				left := 2*i + 1
				right := 2*i + 2
				largest := i
				if left < len(nearest) && nearest[left].dist > nearest[largest].dist {
					largest = left
				}
				if right < len(nearest) && nearest[right].dist > nearest[largest].dist {
					largest = right
				}
				if largest == i {
					break
				}
				nearest[i], nearest[largest] = nearest[largest], nearest[i]
				i = largest
			}
		}
	}

	type cand struct {
		dist  uint64
		label string
	}
	items := make([]cand, 0, k)

	for _, p := range nearest {
		byteOff := s.index.PartitionOffset(p.idx)
		vecStart := byteOff / ivfDimCount
		vecCount := s.index.PartitionVectorCount(p.idx)

		for i := uint32(0); i < vecCount; i++ {
			vi := vecStart + i
			vec := s.index.VectorAt(vi)
			d := dist14Uint8(qUint8, vec)
			lbl := labelStr(s.index.LabelAt(vi))

			if len(items) < k {
				pos := len(items)
				for pos > 0 && items[pos-1].dist > d {
					pos--
				}
				items = append(items, cand{})
				copy(items[pos+1:], items[pos:])
				items[pos] = cand{dist: d, label: lbl}
			} else if d < items[len(items)-1].dist {
				pos := len(items) - 1
				for pos > 0 && items[pos-1].dist > d {
					pos--
				}
				copy(items[pos+1:], items[pos:len(items)-1])
				items[pos] = cand{dist: d, label: lbl}
			}
		}
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no candidates found in index")
	}

	labels := make([]string, len(items))
	for i, c := range items {
		labels[i] = c.label
	}
	return labels, nil
}

// Search finds the k nearest neighbors using the IVF index.
func (s *IVFSearcher) Search(query [14]float64, k int) ([]reference.Reference, error) {
	if k <= 0 {
		return nil, fmt.Errorf("k must be positive, got %d", k)
	}
	totalVectors := int(s.index.VectorCount())
	if totalVectors == 0 {
		return nil, fmt.Errorf("index is empty")
	}

	qUint8 := reference.QuantizeVector(query)
	nParts := int(s.index.PartitionCount())

	type nc struct {
		idx  int
		dist float64
	}
	nearest := make([]nc, 0, s.probeCount)
	for p := 0; p < nParts; p++ {
		c := s.index.Centroid(p)
		d := dist14Centroid(query, c)
		if len(nearest) < s.probeCount {
			nearest = append(nearest, nc{idx: p, dist: d})
			i := len(nearest) - 1
			for i > 0 {
				parent := (i - 1) / 2
				if nearest[parent].dist >= nearest[i].dist {
					break
				}
				nearest[parent], nearest[i] = nearest[i], nearest[parent]
				i = parent
			}
		} else if d < nearest[0].dist {
			nearest[0] = nc{idx: p, dist: d}
			i := 0
			for {
				left := 2*i + 1
				right := 2*i + 2
				largest := i
				if left < len(nearest) && nearest[left].dist > nearest[largest].dist {
					largest = left
				}
				if right < len(nearest) && nearest[right].dist > nearest[largest].dist {
					largest = right
				}
				if largest == i {
					break
				}
				nearest[i], nearest[largest] = nearest[largest], nearest[i]
				i = largest
			}
		}
	}

	type cand struct {
		ref  reference.Reference
		dist uint64
	}
	items := make([]cand, 0, k)

	for _, p := range nearest {
		byteOff := s.index.PartitionOffset(p.idx)
		vecStart := byteOff / ivfDimCount
		vecCount := s.index.PartitionVectorCount(p.idx)

		for i := uint32(0); i < vecCount; i++ {
			vi := vecStart + i
			vec := s.index.VectorAt(vi)
			d := dist14Uint8(qUint8, vec)

			ref := reference.Reference{
				Vector: dequantizeVector(vec),
				Label:  labelStr(s.index.LabelAt(vi)),
			}

			if len(items) < k {
				pos := len(items)
				for pos > 0 && items[pos-1].dist > d {
					pos--
				}
				items = append(items, cand{})
				copy(items[pos+1:], items[pos:])
				items[pos] = cand{ref: ref, dist: d}
			} else if d < items[len(items)-1].dist {
				pos := len(items) - 1
				for pos > 0 && items[pos-1].dist > d {
					pos--
				}
				copy(items[pos+1:], items[pos:len(items)-1])
				items[pos] = cand{ref: ref, dist: d}
			}
		}
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no candidates found in index")
	}

	result := make([]reference.Reference, len(items))
	for i, c := range items {
		result[i] = c.ref
	}
	return result, nil
}

func dist14Centroid(query [14]float64, c [14]float32) float64 {
	d0 := query[0] - float64(c[0])
	d1 := query[1] - float64(c[1])
	d2 := query[2] - float64(c[2])
	d3 := query[3] - float64(c[3])
	d4 := query[4] - float64(c[4])
	d5 := query[5] - float64(c[5])
	d6 := query[6] - float64(c[6])
	d7 := query[7] - float64(c[7])
	d8 := query[8] - float64(c[8])
	d9 := query[9] - float64(c[9])
	d10 := query[10] - float64(c[10])
	d11 := query[11] - float64(c[11])
	d12 := query[12] - float64(c[12])
	d13 := query[13] - float64(c[13])
	return d0*d0 + d1*d1 + d2*d2 + d3*d3 + d4*d4 + d5*d5 + d6*d6 +
		d7*d7 + d8*d8 + d9*d9 + d10*d10 + d11*d11 + d12*d12 + d13*d13
}

func dist14Uint8(a, b [14]uint8) uint64 {
	diff0 := int64(a[0]) - int64(b[0])
	diff1 := int64(a[1]) - int64(b[1])
	diff2 := int64(a[2]) - int64(b[2])
	diff3 := int64(a[3]) - int64(b[3])
	diff4 := int64(a[4]) - int64(b[4])
	diff5 := int64(a[5]) - int64(b[5])
	diff6 := int64(a[6]) - int64(b[6])
	diff7 := int64(a[7]) - int64(b[7])
	diff8 := int64(a[8]) - int64(b[8])
	diff9 := int64(a[9]) - int64(b[9])
	diff10 := int64(a[10]) - int64(b[10])
	diff11 := int64(a[11]) - int64(b[11])
	diff12 := int64(a[12]) - int64(b[12])
	diff13 := int64(a[13]) - int64(b[13])
	return uint64(diff0*diff0) + uint64(diff1*diff1) + uint64(diff2*diff2) +
		uint64(diff3*diff3) + uint64(diff4*diff4) + uint64(diff5*diff5) +
		uint64(diff6*diff6) + uint64(diff7*diff7) + uint64(diff8*diff8) +
		uint64(diff9*diff9) + uint64(diff10*diff10) + uint64(diff11*diff11) +
		uint64(diff12*diff12) + uint64(diff13*diff13)
}

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
		return fraudLabel
	}
	return "legit"
}
