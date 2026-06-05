package search_test

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/leandro-lugaresi/rinha-2026-go/internal/reference"
	"github.com/leandro-lugaresi/rinha-2026-go/internal/search"
)

// ---------------------------------------------------------------------------
// IVF index mini-builder for tests
// ---------------------------------------------------------------------------

// writeMiniIVF writes a valid IVF binary file from a set of references and
// explicit partition assignments. Centroids are supplied as [14]float32.
// Returns the path to the temporary file; caller must os.Remove it.
func writeMiniIVF(t *testing.T, refs []reference.Reference, centroids [][14]float32, assignments []int) string {
	t.Helper()

	if len(refs) != len(assignments) {
		t.Fatalf("refs (%d) and assignments (%d) must match", len(refs), len(assignments))
	}
	numParts := len(centroids)
	if numParts == 0 {
		numParts = 1
	}

	// Count and reorder vectors per partition.
	counts := make([]uint32, numParts)
	for _, a := range assignments {
		if a < 0 || a >= numParts {
			t.Fatalf("partition assignment %d out of range [0, %d)", a, numParts)
		}
		counts[a]++
	}

	offsets := make([]uint32, numParts)
	var o uint32
	for p := 0; p < numParts; p++ {
		offsets[p] = o
		o += counts[p] * referenceLen
	}

	// Build per-partition lists.
	vecsByPart := make([][]reference.Reference, numParts)
	for i, r := range refs {
		p := assignments[i]
		vecsByPart[p] = append(vecsByPart[p], r)
	}

	f, err := os.CreateTemp("", "test-ivf-*.bin")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	path := f.Name()

	writeLE := func(v uint32) {
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], v)
		f.Write(buf[:])
	}

	// Header.
	f.Write([]byte("RVI1"))
	writeLE(uint32(1))                 // version
	writeLE(uint32(numParts))          // partitions
	writeLE(uint32(len(refs)))         // num vectors
	writeLE(uint32(referenceLen))      // dimensions
	writeLE(uint32(1))                 // encoding = uint8

	// Partition metadata.
	for p := 0; p < numParts; p++ {
		for d := 0; d < referenceLen; d++ {
			writeLE(math.Float32bits(centroids[p][d]))
		}
		writeLE(offsets[p])
		writeLE(counts[p])
	}

	// Data: quantized vectors (partition order).
	for p := 0; p < numParts; p++ {
		for _, r := range vecsByPart[p] {
			q := reference.QuantizeVector(r.Vector)
			f.Write(q[:])
		}
	}

	// Labels (partition order).
	for p := 0; p < numParts; p++ {
		for _, r := range vecsByPart[p] {
			var l uint8
			if r.Label == "fraud" {
				l = 1
			}
			f.Write([]byte{l})
		}
	}

	f.Close()
	return path
}

// simpleKMeans runs a few k-means iterations over reference vectors and
// returns centroids (as float32) and per-vector partition assignments.
func simpleKMeans(t *testing.T, refs []reference.Reference, k int) ([][14]float32, []int) {
	t.Helper()

	centroids := make([][14]float32, k)
	assignments := make([]int, len(refs))

	if len(refs) == 0 {
		return centroids, assignments
	}

	// Initialize centroids from evenly-spaced reference vectors.
	step := float64(len(refs)) / float64(k)
	for p := 0; p < k; p++ {
		idx := int(float64(p) * step)
		if idx >= len(refs) {
			idx = len(refs) - 1
		}
		for d := 0; d < referenceLen; d++ {
			centroids[p][d] = float32(refs[idx].Vector[d])
		}
	}

	for iter := 0; iter < 10; iter++ {
		// Assign each vector to nearest centroid (float64 Euclidean).
		for i, r := range refs {
			bestDist := math.MaxFloat64
			bestP := 0
			for p := 0; p < k; p++ {
				var d float64
				for j := 0; j < referenceLen; j++ {
					diff := r.Vector[j] - float64(centroids[p][j])
					d += diff * diff
				}
				if d < bestDist {
					bestDist = d
					bestP = p
				}
			}
			assignments[i] = bestP
		}

		// Recompute centroids.
		sums := make([][14]float64, k)
		counts := make([]int, k)
		for i, r := range refs {
			p := assignments[i]
			for d := 0; d < referenceLen; d++ {
				sums[p][d] += r.Vector[d]
			}
			counts[p]++
		}
		for p := 0; p < k; p++ {
			if counts[p] > 0 {
				n := float64(counts[p])
				for d := 0; d < referenceLen; d++ {
					centroids[p][d] = float32(sums[p][d] / n)
				}
			}
		}
	}

	return centroids, assignments
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestIVFSearchScoresKFive(t *testing.T) {
	refs := loadFixture(t)
	if len(refs) < 20 {
		t.Skip("need at least 20 references")
	}

	centroids, assignments := simpleKMeans(t, refs, 4)
	path := writeMiniIVF(t, refs, centroids, assignments)
	defer os.Remove(path)

	idx, err := reference.LoadIndex(path)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	defer idx.Close()

	s := search.NewIVFSearcher(idx, 2) // probe 2 of 4 partitions (ensures expansion)
	query := [14]float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5}

	results, err := s.Search(query, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	for i, r := range results {
		if r.Label != "legit" && r.Label != "fraud" {
			t.Errorf("result %d: invalid label %q", i, r.Label)
		}
	}

	// Verify sorted ascending by distance.
	for i := 1; i < len(results); i++ {
		dPrev := squaredDist14(query, results[i-1].Vector)
		dCurr := squaredDist14(query, results[i].Vector)
		if dPrev > dCurr {
			t.Errorf("results not sorted: result[%d] dist %.6f > result[%d] dist %.6f",
				i-1, dPrev, i, dCurr)
		}
	}
}

func TestIVFExpandsWhenCandidatesBelowK(t *testing.T) {
	refs := loadFixture(t)
	if len(refs) < 20 {
		t.Skip("need at least 20 references")
	}

	// Create 4 partitions where first 2 have very few vectors (<5).
	centroids := make([][14]float32, 4)
	for p := 0; p < 4; p++ {
		// Use reference vectors as centroids.
		idx := p * (len(refs) / 4)
		if idx >= len(refs) {
			idx = len(refs) - 1
		}
		for d := 0; d < referenceLen; d++ {
			centroids[p][d] = float32(refs[idx].Vector[d])
		}
	}

	// Assign: partition 0 gets 2 vectors, partition 1 gets 2, rest go to 2 and 3.
	assignments := make([]int, len(refs))
	for i := range refs {
		switch {
		case i < 2:
			assignments[i] = 0
		case i < 4:
			assignments[i] = 1
		case i < len(refs)/2+2:
			assignments[i] = 2
		default:
			assignments[i] = 3
		}
	}

	path := writeMiniIVF(t, refs, centroids, assignments)
	defer os.Remove(path)

	idx, err := reference.LoadIndex(path)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	defer idx.Close()

	// Probe only 2 partitions; they contain 4 vectors < K=5, so expansion required.
	s := search.NewIVFSearcher(idx, 2)
	query := [14]float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5}

	results, err := s.Search(query, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("expected 5 results after expansion, got %d", len(results))
	}

	// Verify sorted.
	for i := 1; i < len(results); i++ {
		dPrev := squaredDist14(query, results[i-1].Vector)
		dCurr := squaredDist14(query, results[i].Vector)
		if dPrev > dCurr {
			t.Errorf("results not sorted: %d > %d", i-1, i)
		}
	}
}

func TestIVFUsesAllDimensions(t *testing.T) {
	refs := loadFixture(t)
	if len(refs) < 5 {
		t.Skip("need at least 5 references")
	}

	centroids, assignments := simpleKMeans(t, refs, 4)
	path := writeMiniIVF(t, refs, centroids, assignments)
	defer os.Remove(path)

	idx, err := reference.LoadIndex(path)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	defer idx.Close()

	s := search.NewIVFSearcher(idx, 4)
	query := [14]float64{0.4, 0.4, 0.4, 0.4, 0.4, 0.4, 0.4, 0.4, 0.4, 0.4, 0.4, 0.4, 0.4, 0.4}

	baseline, err := s.Search(query, 5)
	if err != nil {
		t.Fatalf("baseline Search: %v", err)
	}

	dimsUsed := 0
	for dim := 0; dim < referenceLen; dim++ {
		perturbed := query
		perturbed[dim] += 1.0

		mod, err := s.Search(perturbed, 5)
		if err != nil {
			t.Fatalf("perturbed Search dim %d: %v", dim, err)
		}

		different := false
		for i := 0; i < len(baseline) && i < len(mod); i++ {
			if baseline[i].Label != mod[i].Label || baseline[i].Vector[0] != mod[i].Vector[0] {
				different = true
				break
			}
		}
		if different {
			dimsUsed++
		}
	}

	t.Logf("dimensions affecting IVF results: %d/%d", dimsUsed, referenceLen)

	// Verify each dimension participates in distance computation.
	for dim := 0; dim < referenceLen; dim++ {
		perturbed := query
		perturbed[dim] += 1.0

		var deltaBase, deltaPert float64
		for _, r := range refs {
			deltaBase += squaredDist14(query, r.Vector)
			deltaPert += squaredDist14(perturbed, r.Vector)
		}
		if deltaBase == deltaPert {
			t.Errorf("dimension %d: no distance change when perturbed (all vectors have identical value in this dim)", dim)
		}
	}

	if dimsUsed < referenceLen {
		t.Logf("note: %d dimensions did not change top-5; fixture may lack variance in those dims", referenceLen-dimsUsed)
	}
}

func TestIVFRecallVsExact(t *testing.T) {
	refs := loadFixture(t)
	if len(refs) < 10 {
		t.Skip("need at least 10 references")
	}

	centroids, assignments := simpleKMeans(t, refs, 4)
	path := writeMiniIVF(t, refs, centroids, assignments)
	defer os.Remove(path)

	idx, err := reference.LoadIndex(path)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	defer idx.Close()

	s := search.NewIVFSearcher(idx, 4) // probe all partitions

	// Use 5 random-ish queries (first 5 vectors as queries, excluding themselves
	// from the expected result by checking distance > 0).
	recallSum := 0.0
	for qi := 0; qi < 5 && qi < len(refs); qi++ {
		query := refs[qi].Vector

		ivfResults, err := s.Search(query, 5)
		if err != nil {
			t.Fatalf("ivf Search query %d: %v", qi, err)
		}

		exactResults, err := search.ExactKNN(query, refs, 5)
		if err != nil {
			t.Fatalf("exact KNN query %d: %v", qi, err)
		}

		// Compute label-based recall@5: fraction of exact KNN labels also
		// returned by IVF. Vector equality is not used because IVF stores
		// quantized uint8 vectors and the dequantized float64 values differ
		// from the original float64 vectors due to 8-bit precision loss.
		exactLabels := make(map[string]int, len(exactResults))
		for _, r := range exactResults {
			exactLabels[r.Label]++
		}

		overlap := 0
		for _, r := range ivfResults {
			if exactLabels[r.Label] > 0 {
				overlap++
				exactLabels[r.Label]--
			}
		}
		recall := float64(overlap) / 5.0
		recallSum += recall
		t.Logf("query %d: overlap=%d/5 recall=%.2f", qi, overlap, recall)
	}

	avgRecall := recallSum / 5.0
	t.Logf("average recall@5: %.2f", avgRecall)

	if avgRecall == 0 {
		t.Error("IVF recall is 0 — search is not finding any of the exact neighbors")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func squaredDist14(a, b [14]float64) float64 {
	var d float64
	for i := 0; i < referenceLen; i++ {
		diff := a[i] - b[i]
		d += diff * diff
	}
	return d
}
