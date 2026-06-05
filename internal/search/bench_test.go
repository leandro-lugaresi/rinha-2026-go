package search_test

import (
	"fmt"
	"math/rand/v2"
	"os"
	"testing"

	"github.com/leandro-lugaresi/rinha-2026-go/internal/reference"
	"github.com/leandro-lugaresi/rinha-2026-go/internal/search"
)

const indexFilePath = "../../var/references.ivf"

// randomQuery generates a 14-dimensional vector with values in [-1, 1].
// Dimensions 0-4 and 7-13 are clamped to [0, 1]; dimensions 5-6 may be -1.
func randomQuery(rng *rand.Rand) [14]float64 {
	var q [14]float64
	for i := 0; i < 14; i++ {
		switch i {
		case 5, 6:
			q[i] = rng.Float64()*2 - 1
		default:
			q[i] = rng.Float64()
		}
	}
	return q
}

func BenchmarkIVFSearch(b *testing.B) {
	idx := loadIVFIndex(b)
	if idx == nil {
		b.Skip("IVF index not found")
	}
	defer idx.Close()

	s := search.NewIVFSearcher(idx, 0) // uses env or default 32

	rng := rand.New(rand.NewPCG(1, 2))
	queries := make([][14]float64, b.N)
	for i := range queries {
		queries[i] = randomQuery(rng)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results, err := s.Search(queries[i], 5)
		if err != nil {
			b.Fatalf("Search: %v", err)
		}
		if len(results) != 5 {
			b.Fatalf("expected 5 results, got %d", len(results))
		}
	}
}

func BenchmarkExactSearch(b *testing.B) {
	refs := loadBenchmarkRefs(b)
	if refs == nil {
		b.Skip("example references not found")
	}

	rng := rand.New(rand.NewPCG(1, 2))
	queries := make([][14]float64, b.N)
	for i := range queries {
		queries[i] = randomQuery(rng)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results, err := search.ExactKNN(queries[i], refs, 5)
		if err != nil {
			b.Fatalf("ExactKNN: %v", err)
		}
		if len(results) != 5 {
			b.Fatalf("expected 5 results, got %d", len(results))
		}
	}
}

func loadIVFIndex(tb testing.TB) *reference.IVFIndex {
	tb.Helper()
	if _, err := os.Stat(indexFilePath); os.IsNotExist(err) {
		return nil
	}
	idx, err := reference.LoadIndex(indexFilePath)
	if err != nil {
		tb.Fatalf("LoadIndex: %v", err)
	}
	fmt.Fprintf(os.Stderr, "IVF index: %d vectors, %d partitions\n",
		idx.VectorCount(), idx.PartitionCount())
	return idx
}

func loadBenchmarkRefs(tb testing.TB) []reference.Reference {
	tb.Helper()
	fixturePath := "../../.context/rinha-de-backend-2026/resources/example-references.json"
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		return nil
	}
	refs, err := reference.LoadExampleReferences(fixturePath)
	if err != nil {
		tb.Fatalf("LoadExampleReferences: %v", err)
	}
	return refs
}
