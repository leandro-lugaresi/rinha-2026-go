// Package search provides brute-force and approximate nearest-neighbor
// search over labeled reference vectors for fraud detection.
package search

import (
	"fmt"
	"sort"

	"github.com/leandro-lugaresi/rinha-2026-go/internal/reference"
)

// ExactKNN performs an exact brute-force K-nearest-neighbor search using
// squared Euclidean distance over all 14 dimensions. No sqrt is applied
// because monotonic ordering is preserved without it.
//
// Returns up to k nearest references sorted by distance ascending. An
// error is returned if k <= 0, if references is empty, or if k exceeds
// the number of references.
func ExactKNN(query [14]float64, references []reference.Reference, k int) ([]reference.Reference, error) {
	if k <= 0 {
		return nil, fmt.Errorf("k must be positive, got %d", k)
	}
	if len(references) == 0 {
		return nil, fmt.Errorf("no references provided")
	}
	if k > len(references) {
		return nil, fmt.Errorf("k (%d) exceeds number of references (%d)", k, len(references))
	}

	type scored struct {
		ref  reference.Reference
		dist float64
	}

	scores := make([]scored, len(references))
	for i, ref := range references {
		var d float64
		v := ref.Vector
		for j := 0; j < 14; j++ {
			diff := query[j] - v[j]
			d += diff * diff
		}
		scores[i] = scored{ref: ref, dist: d}
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].dist < scores[j].dist
	})

	result := make([]reference.Reference, k)
	for i := 0; i < k; i++ {
		result[i] = scores[i].ref
	}

	return result, nil
}
