package main

import "testing"

func TestAssignNearestMatchesReference(t *testing.T) {
	const (
		numVectors = 5_000
		k          = coarsePartitions
	)
	vectors := makeBenchmarkVectors(numVectors)
	centroids := make([][dimCount]float32, k)
	for c := 0; c < k; c++ {
		base := c * dimCount
		copy(centroids[c][:], vectors[base:base+dimCount])
	}

	got := assignNearest(vectors, numVectors, centroids)
	want := assignNearestReference(vectors, numVectors, centroids)
	for i := 0; i < numVectors; i++ {
		if got[i] != want[i] {
			t.Fatalf("assignment[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func BenchmarkAssignNearest(b *testing.B) {
	const (
		numVectors = 100_000
		k          = coarsePartitions
	)
	vectors := makeBenchmarkVectors(numVectors)
	centroids := make([][dimCount]float32, k)
	for c := 0; c < k; c++ {
		base := c * dimCount
		copy(centroids[c][:], vectors[base:base+dimCount])
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = assignNearest(vectors, numVectors, centroids)
	}
}

func BenchmarkAssignParallel(b *testing.B) {
	const (
		numVectors = 100_000
		k          = coarsePartitions
	)
	vectors := makeBenchmarkVectors(numVectors)
	centroids := make([][dimCount]float32, k)
	out := make([]int, numVectors)
	for c := 0; c < k; c++ {
		base := c * dimCount
		copy(centroids[c][:], vectors[base:base+dimCount])
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		assignParallel(vectors, numVectors, k, centroids, out)
	}
}

func makeBenchmarkVectors(numVectors int) []float32 {
	vectors := make([]float32, numVectors*dimCount)
	for i := 0; i < numVectors; i++ {
		base := i * dimCount
		for j := 0; j < dimCount; j++ {
			vectors[base+j] = float32((i*j+j)%251) / 250
		}
	}
	return vectors
}

func assignNearestReference(vectors []float32, numVectors int, centroids [][dimCount]float32) []int {
	out := make([]int, numVectors)
	for i := 0; i < numVectors; i++ {
		base := i * dimCount
		bestDist := float32(1<<32 - 1)
		bestC := 0
		for c := 0; c < len(centroids); c++ {
			var dist float32
			for j := 0; j < dimCount; j++ {
				diff := vectors[base+j] - centroids[c][j]
				dist += diff * diff
			}
			if dist < bestDist {
				bestDist = dist
				bestC = c
			}
		}
		out[i] = bestC
	}
	return out
}
