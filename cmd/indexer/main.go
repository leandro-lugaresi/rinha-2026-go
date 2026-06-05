package main

import (
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand/v2"
	"os"
	"runtime"
	"sync"
)

const (
	magicBytes          = "RVI1"
	version             = uint32(1)
	dimCount            = 14
	encodingUint8       = uint32(1)
	defaultPartitions   = 8192
	coarsePartitions    = 128
	finePerCoarse       = 64
	kmeansIterations    = 5
	convergenceEpsilon  = 1e-6
	sentinelQuantized   = uint8(255)
)

func main() {
	log.SetFlags(0)

	inputPath := flag.String("input", "", "Path to references.json.gz")
	outputPath := flag.String("output", "", "Output path for the IVF binary index")
	numPartitions := flag.Int("partitions", defaultPartitions, "Number of IVF partitions (must be 8192 or 128×64)")
	flag.Parse()

	if *inputPath == "" || *outputPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: indexer -input <references.json.gz> -output <output.ivf>\n")
		os.Exit(1)
	}

	if *numPartitions != defaultPartitions {
		log.Fatalf("unsupported partition count %d: only %d is supported", *numPartitions, defaultPartitions)
	}

	log.Printf("Loading references from %s", *inputPath)
	vectors, labels, err := parseGzippedJSON(*inputPath)
	if err != nil {
		log.Fatalf("parse references: %v", err)
	}
	numVectors := len(labels)
	log.Printf("Loaded %d vectors (%.1f MB float32)", numVectors, float64(numVectors*dimCount*4)/(1024*1024))

	// Start RSS monitoring in background.
	peakRSS := monitorRSS()

	log.Printf("Training hierarchical k-means (%d → %d × %d → %d partitions)",
		coarsePartitions, coarsePartitions, finePerCoarse, defaultPartitions)

	assignments, centroids := trainHierarchical(vectors, numVectors)

	log.Printf("Quantizing and reordering vectors...")
	_ = peakRSS() // sample peak

	if err := writeIndexFile(*outputPath, vectors, labels, assignments, centroids, numVectors); err != nil {
		log.Fatalf("write index: %v", err)
	}

	fi, err := os.Stat(*outputPath)
	if err != nil {
		log.Fatalf("stat output: %v", err)
	}

	peakBytes := peakRSS()
	log.Printf("Done. Output: %s (%d bytes, %.1f MB)", *outputPath, fi.Size(), float64(fi.Size())/(1024*1024))
	log.Printf("Peak RSS: %.1f MB", float64(peakBytes)/(1024*1024))
}

// rawRef is the JSON shape of a single reference entry in the gzip stream.
type rawRef struct {
	Vector []float64 `json:"vector"`
	Label  string    `json:"label"`
}

// parseGzippedJSON streams a gzipped JSON array of references, returning
// flat float32 vectors and uint8 labels (0=legit, 1=fraud).
func parseGzippedJSON(path string) ([]float32, []uint8, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	dec := json.NewDecoder(gz)

	// Expect opening '['
	if t, err := dec.Token(); err != nil || t != json.Delim('[') {
		return nil, nil, fmt.Errorf("expected JSON array start, got token %v (err=%v)", t, err)
	}

	// Pre-allocate for 3M vectors.
	vectors := make([]float32, 0, 3_000_000*dimCount)
	labels := make([]uint8, 0, 3_000_000)

	for dec.More() {
		var r rawRef
		if err := dec.Decode(&r); err != nil {
			return nil, nil, fmt.Errorf("decode reference at position %d: %w", len(labels), err)
		}
		if len(r.Vector) != dimCount {
			return nil, nil, fmt.Errorf("reference %d: expected 14 dims, got %d", len(labels), len(r.Vector))
		}
		for _, v := range r.Vector {
			vectors = append(vectors, float32(v))
		}
		var lbl uint8
		if r.Label == "fraud" {
			lbl = 1
		}
		labels = append(labels, lbl)
	}

	// Expect closing ']'
	if t, err := dec.Token(); err != nil || t != json.Delim(']') {
		return nil, nil, fmt.Errorf("expected JSON array end, got token %v (err=%v)", t, err)
	}

	return vectors, labels, nil
}

// trainHierarchical runs two-level k-means: first k=coarsePartitions, then
// within each coarse cluster k=finePerCoarse, producing 8192 centroids
// total. Returns partition assignments (0..8191) and the 8192 centroids.
func trainHierarchical(vectors []float32, numVectors int) ([]uint32, [][dimCount]float32) {
	// Phase 1: coarse clustering (k=128).
	log.Printf("  Phase 1: clustering %d vectors into %d coarse partitions", numVectors, coarsePartitions)
	coarseCentroids := kmeansFlat(vectors, numVectors, coarsePartitions)
	coarseAssign := assignNearest(vectors, numVectors, coarseCentroids)

	// Phase 2: within each coarse cluster, cluster into finePerCoarse partitions.
	log.Printf("  Phase 2: clustering within each coarse partition into %d fine partitions", finePerCoarse)
	centroids := make([][dimCount]float32, defaultPartitions)
	assignments := make([]uint32, numVectors)

	// Gather indices per coarse cluster.
	coarseVecs := make([][]int, coarsePartitions)
	for i := 0; i < numVectors; i++ {
		c := coarseAssign[i]
		coarseVecs[c] = append(coarseVecs[c], i)
	}

	fineIdx := 0
	for c := 0; c < coarsePartitions; c++ {
		indices := coarseVecs[c]
		if len(indices) == 0 {
			// Empty coarse cluster: leave centroids at zero.
			for k := 0; k < finePerCoarse; k++ {
				fineIdx++
			}
			continue
		}

		// Extract sub-vectors for this coarse cluster.
		subVectors := make([]float32, len(indices)*dimCount)
		for si, idx := range indices {
			base := si * dimCount
			copy(subVectors[base:base+dimCount], vectors[idx*dimCount:idx*dimCount+dimCount])
		}

		fineCentroids := kmeansFlat(subVectors, len(indices), finePerCoarse)
		fineAssign := assignNearest(subVectors, len(indices), fineCentroids)

		for si, idx := range indices {
			assignments[idx] = uint32(fineIdx + fineAssign[si])
		}
		for k := 0; k < finePerCoarse; k++ {
			centroids[fineIdx+k] = fineCentroids[k]
		}
		fineIdx += finePerCoarse
	}

	return assignments, centroids
}

// kmeansFlat runs Lloyd's algorithm with k clusters on the given vectors.
// Returns the final centroids.
func kmeansFlat(vectors []float32, numVectors, k int) [][dimCount]float32 {
	if numVectors == 0 {
		centroids := make([][dimCount]float32, k)
		return centroids
	}

	rng := rand.New(rand.NewPCG(42, 0))

	// Initialize centroids via random selection.
	centroids := make([][dimCount]float32, k)
	if numVectors <= k {
		for i := 0; i < numVectors; i++ {
			base := i * dimCount
			for j := 0; j < dimCount; j++ {
				centroids[i][j] = vectors[base+j]
			}
		}
		return centroids
	}

	// Reservoir sampling for initial centroids.
	copyCentroids := make([]int, k)
	for i := 0; i < k; i++ {
		copyCentroids[i] = i
	}
	for i := k; i < numVectors; i++ {
		j := rng.IntN(i + 1)
		if j < k {
			copyCentroids[j] = i
		}
	}
	for i := 0; i < k; i++ {
		base := copyCentroids[i] * dimCount
		copy(centroids[i][:], vectors[base:base+dimCount])
	}

	assignBuf := make([]int, numVectors)
	sums := make([][]float32, k)
	for i := range sums {
		sums[i] = make([]float32, dimCount)
	}
	counts := make([]int, k)

	for iter := 0; iter < kmeansIterations; iter++ {
		// Assign each vector to nearest centroid.
		assignParallel(vectors, numVectors, k, centroids, assignBuf)

		// Reset accumulators.
		for i := range sums {
			for j := range sums[i] {
				sums[i][j] = 0
			}
			counts[i] = 0
		}

		// Accumulate.
		for i := 0; i < numVectors; i++ {
			c := assignBuf[i]
			base := i * dimCount
			for j := 0; j < dimCount; j++ {
				sums[c][j] += vectors[base+j]
			}
			counts[c]++
		}

		// Recompute centroids, track max movement.
		maxDelta := float32(0)
		for c := 0; c < k; c++ {
			if counts[c] == 0 {
				continue
			}
			n := float32(counts[c])
			for j := 0; j < dimCount; j++ {
				newVal := sums[c][j] / n
				delta := float32(math.Abs(float64(newVal - centroids[c][j])))
				if delta > maxDelta {
					maxDelta = delta
				}
				centroids[c][j] = newVal
			}
		}

		if maxDelta < convergenceEpsilon {
			break
		}
	}

	return centroids
}

// assignParallel assigns each vector to its nearest centroid using all CPUs.
func assignParallel(vectors []float32, numVectors, k int, centroids [][dimCount]float32, out []int) {
	// Pre-compute centroid squared norms.
	centroidSqNorms := make([]float32, k)
	for c := 0; c < k; c++ {
		var s float32
		for j := 0; j < dimCount; j++ {
			s += centroids[c][j] * centroids[c][j]
		}
		centroidSqNorms[c] = s
	}

	numCPU := runtime.GOMAXPROCS(0)
	chunk := (numVectors + numCPU - 1) / numCPU

	done := make(chan struct{}, numCPU)
	for w := 0; w < numCPU; w++ {
		go func(worker int) {
			defer func() { done <- struct{}{} }()
			start := worker * chunk
			if start >= numVectors {
				return
			}
			end := start + chunk
			if end > numVectors {
				end = numVectors
			}
			for i := start; i < end; i++ {
				base := i * dimCount
				bestDist := float32(math.MaxFloat32)
				bestC := 0
				for c := 0; c < k; c++ {
					var dot float32
					cent := centroids[c]
					for j := 0; j < dimCount; j++ {
						dot += vectors[base+j] * cent[j]
					}
					dist := centroidSqNorms[c] - 2*dot
					if dist < bestDist {
						bestDist = dist
						bestC = c
					}
				}
				out[i] = bestC
			}
		}(w)
	}
	for w := 0; w < numCPU; w++ {
		<-done
	}
}

// assignNearest is like assignParallel but single-threaded and returns
// a new slice. Used after k-means for final assignment.
func assignNearest(vectors []float32, numVectors int, centroids [][dimCount]float32) []int {
	k := len(centroids)
	centroidSqNorms := make([]float32, k)
	for c := 0; c < k; c++ {
		var s float32
		for j := 0; j < dimCount; j++ {
			s += centroids[c][j] * centroids[c][j]
		}
		centroidSqNorms[c] = s
	}

	out := make([]int, numVectors)
	for i := 0; i < numVectors; i++ {
		base := i * dimCount
		bestDist := float32(math.MaxFloat32)
		bestC := 0
		for c := 0; c < k; c++ {
			var dot float32
			cent := centroids[c]
			for j := 0; j < dimCount; j++ {
				dot += vectors[base+j] * cent[j]
			}
			dist := centroidSqNorms[c] - 2*dot
			if dist < bestDist {
				bestDist = dist
				bestC = c
			}
		}
		out[i] = bestC
	}
	return out
}

// writeIndexFile writes the binary IVF index to path.
func writeIndexFile(path string, vectors []float32, labels []uint8, assignments []uint32, centroids [][dimCount]float32, numVectors int) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer f.Close()

	bw := newBufWriter(f)

	// Compute partition sizes and offsets.
	partCounts := make([]uint32, defaultPartitions)
	for _, a := range assignments {
		partCounts[a]++
	}

	partOffsets := make([]uint32, defaultPartitions)
	var off uint32
	for p := uint32(0); p < defaultPartitions; p++ {
		partOffsets[p] = off
		off += partCounts[p] * dimCount
	}

	// Reorder vectors by partition.
	qvecs := make([]uint8, numVectors*dimCount)
	qlabels := make([]uint8, numVectors)
	partCursors := make([]uint32, defaultPartitions)
	copy(partCursors, partOffsets)

	for i := 0; i < numVectors; i++ {
		p := assignments[i]
		base := i * dimCount
		dest := int(partCursors[p])
		for j := 0; j < dimCount; j++ {
			v := float64(vectors[base+j])
			var q uint8
			if v < 0 {
				q = sentinelQuantized
			} else {
				q = uint8(math.Round(v * 254))
			}
			qvecs[dest+j] = q
		}
		qlabels[int(partCursors[p]/dimCount)] = labels[i]
		partCursors[p] += dimCount
	}

	// Write header.
	bw.write([]byte(magicBytes))
	bw.writeUint32(version)
	bw.writeUint32(defaultPartitions)
	bw.writeUint32(uint32(numVectors))
	bw.writeUint32(dimCount)
	bw.writeUint32(encodingUint8)

	// Write partition metadata.
	for p := uint32(0); p < defaultPartitions; p++ {
		for j := 0; j < dimCount; j++ {
			bw.writeUint32(math.Float32bits(centroids[p][j]))
		}
		bw.writeUint32(partOffsets[p])
		bw.writeUint32(partCounts[p])
	}

	// Write data section.
	bw.write(qvecs)
	bw.write(qlabels)

	return bw.flush()
}

// monitorRSS starts a background goroutine that samples RSS and returns
// a function that reports the peak observed so far. The first call to the
// returned function stops monitoring and returns the final peak.
func monitorRSS() func() uint64 {
	var peak uint64
	var once sync.Once
	done := make(chan struct{})
	var m runtime.MemStats

	go func() {
		for {
			select {
			case <-done:
				return
			default:
				runtime.ReadMemStats(&m)
				rss := m.Sys
				if rss > peak {
					peak = rss
				}
			}
			runtime.Gosched()
		}
	}()

	return func() uint64 {
		runtime.ReadMemStats(&m)
		if m.Sys > peak {
			peak = m.Sys
		}
		once.Do(func() { close(done) })
		return peak
	}
}

// bufWriter wraps an io.Writer with a buffer and uint32 encoding helpers.
type bufWriter struct {
	w   io.Writer
	buf []byte
}

func newBufWriter(w io.Writer) *bufWriter {
	const bufSize = 64 * 1024
	return &bufWriter{w: w, buf: make([]byte, 0, bufSize)}
}

func (bw *bufWriter) write(p []byte) {
	if len(bw.buf)+len(p) > cap(bw.buf) {
		bw.flushBuf()
		if len(p) >= cap(bw.buf) {
			bw.w.Write(p)
			return
		}
	}
	bw.buf = append(bw.buf, p...)
}

func (bw *bufWriter) writeUint32(v uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	bw.write(buf[:])
}

func (bw *bufWriter) flushBuf() {
	if len(bw.buf) > 0 {
		bw.w.Write(bw.buf)
		bw.buf = bw.buf[:0]
	}
}

func (bw *bufWriter) flush() error {
	bw.flushBuf()
	if f, ok := bw.w.(*os.File); ok {
		return f.Sync()
	}
	return nil
}

