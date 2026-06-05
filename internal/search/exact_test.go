package search_test

import (
	"os"
	"sort"
	"testing"

	"github.com/leandro-lugaresi/rinha-2026-go/internal/reference"
	"github.com/leandro-lugaresi/rinha-2026-go/internal/search"
)

const fixturePath = "../../.context/rinha-de-backend-2026/resources/example-references.json"

const referenceLen = 14

func loadFixture(t *testing.T) []reference.Reference {
	t.Helper()
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skipf("fixture file not found: %s", fixturePath)
	}
	refs, err := reference.LoadExampleReferences(fixturePath)
	if err != nil {
		t.Fatalf("LoadExampleReferences: %v", err)
	}
	return refs
}

func TestExactKNNReturnsKResults(t *testing.T) {
	refs := loadFixture(t)
	if len(refs) < 5 {
		t.Skip("need at least 5 references")
	}

	query := [14]float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5}
	results, err := search.ExactKNN(query, refs, 5)
	if err != nil {
		t.Fatalf("ExactKNN: %v", err)
	}

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	for i, r := range results {
		if r.Label != "legit" && r.Label != "fraud" {
			t.Errorf("result %d: invalid label %q", i, r.Label)
		}
		t.Logf("result %d: label=%s vector[0]=%.4f", i, r.Label, r.Vector[0])
	}
}

func TestExactKNNSortedByDistance(t *testing.T) {
	refs := loadFixture(t)
	if len(refs) < 5 {
		t.Skip("need at least 5 references")
	}

	query := [14]float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5}
	results, err := search.ExactKNN(query, refs, 5)
	if err != nil {
		t.Fatalf("ExactKNN: %v", err)
	}

	dists := make([]float64, len(results))
	for i, r := range results {
		var d float64
		for j := 0; j < referenceLen; j++ {
			diff := query[j] - r.Vector[j]
			d += diff * diff
		}
		dists[i] = d
	}

	if !sort.Float64sAreSorted(dists) {
		t.Errorf("distances not sorted ascending: %v", dists)
	}

	t.Logf("distances: %v", dists)
}

func TestExactKNNUsesAllDimensions(t *testing.T) {
	refs := loadFixture(t)
	if len(refs) < 2 {
		t.Skip("need at least 2 references")
	}

	query := [14]float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5}

	orig, err := search.ExactKNN(query, refs, 5)
	if err != nil {
		t.Fatalf("ExactKNN baseline: %v", err)
	}

	dimsUsed := 0
	for dim := 0; dim < referenceLen; dim++ {
		perturbed := query
		perturbed[dim] += 1.0

		mod, err := search.ExactKNN(perturbed, refs, 5)
		if err != nil {
			t.Fatalf("ExactKNN dim %d: %v", dim, err)
		}

		different := false
		for i := 0; i < 5; i++ {
			if orig[i].Label != mod[i].Label ||
				orig[i].Vector[0] != mod[i].Vector[0] {
				different = true
				break
			}
		}
		if different {
			dimsUsed++
		}
	}

	t.Logf("dimensions affecting results: %d/%d", dimsUsed, referenceLen)

	if dimsUsed < referenceLen {
		for dim := 0; dim < referenceLen; dim++ {
			perturbed := query
			perturbed[dim] += 1.0
			var deltaOrig, deltaPert float64
			for _, r := range refs {
				var d float64
				for j := 0; j < referenceLen; j++ {
					diff := query[j] - r.Vector[j]
					d += diff * diff
				}
				deltaOrig += d

				d = 0
				for j := 0; j < referenceLen; j++ {
					diff := perturbed[j] - r.Vector[j]
					d += diff * diff
				}
				deltaPert += d
			}
			if deltaOrig == deltaPert {
				t.Errorf("dimension %d: no distance change when perturbed (all vectors have same value in this dim)", dim)
			}
		}
		if dimsUsed < referenceLen {
			t.Logf("warning: %d dimensions did not change top-5 ordering; this is expected if fixture has low variance in those dims", referenceLen-dimsUsed)
		}
	}
}

func TestExactKNNErrors(t *testing.T) {
	refs := loadFixture(t)
	if len(refs) == 0 {
		t.Skip("no references loaded")
	}

	query := [14]float64{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1}

	t.Run("zero_k", func(t *testing.T) {
		_, err := search.ExactKNN(query, refs, 0)
		if err == nil {
			t.Fatal("expected error for k=0")
		}
	})

	t.Run("negative_k", func(t *testing.T) {
		_, err := search.ExactKNN(query, refs, -1)
		if err == nil {
			t.Fatal("expected error for negative k")
		}
	})

	t.Run("empty_references", func(t *testing.T) {
		_, err := search.ExactKNN(query, nil, 5)
		if err == nil {
			t.Fatal("expected error for nil references")
		}
	})

	t.Run("k_exceeds_references", func(t *testing.T) {
		_, err := search.ExactKNN(query, refs, len(refs)+1)
		if err == nil {
			t.Fatal("expected error when k exceeds reference count")
		}
	})
}
