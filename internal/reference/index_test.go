package reference

import (
	"encoding/binary"
	"math"
	"os"
	"testing"
)

func TestIVFFileFormat(t *testing.T) {
	path := writeMiniIndex(t, 10, 4)
	defer os.Remove(path)

	idx, err := LoadIndex(path)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	defer idx.Close()

	h := idx.Header()

	if h.Version != 1 {
		t.Errorf("version = %d, want 1", h.Version)
	}
	if h.Dimensions != 14 {
		t.Errorf("dimensions = %d, want 14", h.Dimensions)
	}
	if h.Partitions != 4 {
		t.Errorf("partitions = %d, want 4", h.Partitions)
	}
	if h.NumVectors != 10 {
		t.Errorf("numVectors = %d, want 10", h.NumVectors)
	}
	if h.Encoding != 1 {
		t.Errorf("encoding = %d, want 1", h.Encoding)
	}

	if idx.PartitionCount() != 4 {
		t.Errorf("PartitionCount = %d, want 4", idx.PartitionCount())
	}
	if idx.VectorCount() != 10 {
		t.Errorf("VectorCount = %d, want 10", idx.VectorCount())
	}

	// Verify partition metadata.
	for p := 0; p < 4; p++ {
		c := idx.Centroid(p)
		for d := 0; d < 14; d++ {
			if c[d] != float32(p*10+d) {
				t.Errorf("centroid[%d][%d] = %f, want %f", p, d, c[d], float32(p*10+d))
			}
		}
	}

	if idx.PartitionOffset(0) != 0 {
		t.Errorf("partition[0] offset = %d, want 0", idx.PartitionOffset(0))
	}
	if idx.PartitionVectorCount(0) != 3 {
		t.Errorf("partition[0] count = %d, want 3", idx.PartitionVectorCount(0))
	}

	// Verify vector access.
	v0 := idx.VectorAt(0)
	for d := 0; d < 14; d++ {
		want := uint8(d)
		if v0[d] != want {
			t.Errorf("vector[0][%d] = %d, want %d", d, v0[d], want)
		}
	}

	if idx.LabelAt(0) != 0 {
		t.Errorf("label[0] = %d, want 0", idx.LabelAt(0))
	}
	if idx.LabelAt(9) != 1 {
		t.Errorf("label[9] = %d, want 1", idx.LabelAt(9))
	}
}

func TestQuantizedSentinelRoundTrip(t *testing.T) {
	// -1 in dims 5/6 should map to 255.
	v := [14]float64{
		0.0, 0.5, 1.0, 0.25, 0.75,
		-1, -1, // dims 5, 6: sentinel
		0.1, 0.9, 0, 1, 0, 0.5, 0.0,
	}
	q := QuantizeVector(v)

	if q[5] != 255 {
		t.Errorf("sentinel dim 5: %d, want 255", q[5])
	}
	if q[6] != 255 {
		t.Errorf("sentinel dim 6: %d, want 255", q[6])
	}

	// Normal [0,1] values should map correctly.
	checkQ := func(dim int, f float64, want uint8) {
		t.Helper()
		got := q[dim]
		if got != want {
			t.Errorf("dim %d (%f) → %d, want %d", dim, f, got, want)
		}
	}
	checkQ(0, 0.0, 0)
	checkQ(1, 0.5, 127)
	checkQ(2, 1.0, 254)
	checkQ(9, 0.0, 0)
	checkQ(10, 1.0, 254)

	// Two sentinel vectors should have distance 0 on dims 5/6.
	v2 := [14]float64{
		0.1, 0.6, 0.0, 0.75, 0.25,
		-1, -1,
		0.9, 0.1, 1, 0, 1, 0.5, 0.0,
	}
	q2 := QuantizeVector(v2)
	if q2[5] != 255 || q2[6] != 255 {
		t.Fatalf("sentinel in v2 not encoded: %d, %d", q2[5], q2[6])
	}

	// Integer distance on sentinel dims should be 0.
	diff5 := int(q[5]) - int(q2[5])
	diff6 := int(q[6]) - int(q2[6])
	if diff5 != 0 || diff6 != 0 {
		t.Errorf("sentinel distance: dim5=%d dim6=%d, want 0", diff5, diff6)
	}

	// Distance between sentinel and a real value should be large.
	v3 := [14]float64{
		0.0, 0.0, 0.0, 0.0, 0.0,
		0.5, 0.3, // real values for dims 5/6
		0.0, 0.0, 0, 0, 0, 0.0, 0.0,
	}
	q3 := QuantizeVector(v3)
	diff5Real := int(q[5]) - int(q3[5])
	diff6Real := int(q[6]) - int(q3[6])
	if diff5Real == 0 {
		t.Error("sentinel vs real on dim 5 should have non-zero diff")
	}
	if diff6Real == 0 {
		t.Error("sentinel vs real on dim 6 should have non-zero diff")
	}
}

// writeMiniIndex creates a minimal IVF index file for testing LoadIndex.
func writeMiniIndex(t *testing.T, numVectors, numParts int) string {
	t.Helper()

	f, err := os.CreateTemp("", "test-ivf-*.bin")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	path := f.Name()

	// Header.
	f.Write([]byte("RVI1"))
	writeLE(f, uint32(1))             // version
	writeLE(f, uint32(numParts))      // partitions
	writeLE(f, uint32(numVectors))    // num vectors
	writeLE(f, uint32(14))            // dimensions
	writeLE(f, uint32(1))             // encoding

	// Partition metadata: compute offsets and counts.
	counts := make([]uint32, numParts)
	for i := 0; i < numVectors; i++ {
		counts[i%numParts]++
	}
	offsets := make([]uint32, numParts)
	var off uint32
	for p := 0; p < numParts; p++ {
		offsets[p] = off
		off += counts[p] * 14
	}

	for p := 0; p < numParts; p++ {
		for d := 0; d < 14; d++ {
			writeLE(f, math.Float32bits(float32(p*10+d)))
		}
		writeLE(f, offsets[p])
		writeLE(f, counts[p])
	}

	// Data: quantized vectors.
	for i := 0; i < numVectors; i++ {
		for d := 0; d < 14; d++ {
			f.Write([]byte{uint8(d)})
		}
	}

	// Labels: alternating 0/1, last is 1.
	for i := 0; i < numVectors-1; i++ {
		f.Write([]byte{uint8(i % 2)})
	}
	f.Write([]byte{1})

	f.Close()
	return path
}

func writeLE(f *os.File, v uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	f.Write(buf[:])
}
