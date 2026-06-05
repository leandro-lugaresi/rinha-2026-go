package reference

import (
	"encoding/binary"
	"math"
	"os"
	"testing"
)

func makeBenchIndex(b *testing.B, numVectors, numParts int) string {
	b.Helper()

	f, err := os.CreateTemp("", "bench-ivf-*.bin")
	if err != nil {
		b.Fatalf("create temp: %v", err)
	}

	f.Write([]byte("RVI1"))
	writeLEBench(f, uint32(1))
	writeLEBench(f, uint32(numParts))
	writeLEBench(f, uint32(numVectors))
	writeLEBench(f, uint32(14))
	writeLEBench(f, uint32(1))

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
			writeLEBench(f, math.Float32bits(float32(p*10+d)))
		}
		writeLEBench(f, offsets[p])
		writeLEBench(f, counts[p])
	}

	for i := 0; i < numVectors; i++ {
		for d := 0; d < 14; d++ {
			f.Write([]byte{uint8(d)})
		}
	}

	for i := 0; i < numVectors-1; i++ {
		f.Write([]byte{uint8(i % 2)})
	}
	f.Write([]byte{1})

	if err := f.Close(); err != nil {
		b.Fatalf("close temp: %v", err)
	}
	return f.Name()
}

func writeLEBench(f *os.File, v uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	f.Write(buf[:])
}

func BenchmarkLoadIndex(b *testing.B) {
	path := makeBenchIndex(b, 50000, 32)
	b.Cleanup(func() { os.Remove(path) })

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx, err := LoadIndex(path)
		if err != nil {
			b.Fatalf("LoadIndex: %v", err)
		}
		if err := idx.Close(); err != nil {
			b.Fatalf("Close: %v", err)
		}
	}
}
