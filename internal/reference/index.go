// Package reference provides types and loading for reference vectors used
// in KNN search validation and benchmark correctness checks.
package reference

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"syscall"
)

// Magic bytes identifying an IVF index file. Must be exactly "RVI1".
const ivfMagic = "RVI1"

// Header constants defining the binary layout of an IVF index file.
const (
	ivfHeaderSize        = 24 // 6 × uint32
	ivfPartitionMetaSize = 64 // centroid(56) + offset(4) + count(4)
	ivfDimCount          = 14
	IVFEncodingUint8     = uint32(1)
	ivfSentinelQuantized = uint8(255)
)

// IVFIndex is a memory-mapped Inverted File Index for approximate
// nearest-neighbor search over quantized reference vectors.
type IVFIndex struct {
	data   []byte        // mmap'd file contents
	header ivfHeader     // parsed header fields
	parts  []ivfPartMeta // parsed partition metadata
}

// ivfHeader mirrors the on-disk header layout.
type ivfHeader struct {
	Magic      [4]byte
	Version    uint32
	Partitions uint32
	NumVectors uint32
	Dimensions uint32
	Encoding   uint32
}

// ivfPartMeta mirrors the on-disk partition metadata.
type ivfPartMeta struct {
	Centroid [14]float32
	Offset   uint32
	Count    uint32
}

// IVFFileHeader contains the publicly accessible fields of the IVF index header.
type IVFFileHeader struct {
	Version    uint32
	Partitions uint32
	NumVectors uint32
	Dimensions uint32
	Encoding   uint32
}

// Header returns the parsed header from a loaded index.
func (idx *IVFIndex) Header() IVFFileHeader {
	return IVFFileHeader{
		Version:    idx.header.Version,
		Partitions: idx.header.Partitions,
		NumVectors: idx.header.NumVectors,
		Dimensions: idx.header.Dimensions,
		Encoding:   idx.header.Encoding,
	}
}

// PartitionCount returns the number of partitions in the index.
func (idx *IVFIndex) PartitionCount() uint32 { return idx.header.Partitions }

// VectorCount returns the number of vectors in the index.
func (idx *IVFIndex) VectorCount() uint32 { return idx.header.NumVectors }

// Centroid returns the centroid for partition p.
func (idx *IVFIndex) Centroid(p int) [14]float32 {
	if p < 0 || p >= len(idx.parts) {
		return [14]float32{}
	}
	return idx.parts[p].Centroid
}

// PartitionOffset returns the byte offset into the data section for partition p.
func (idx *IVFIndex) PartitionOffset(p int) uint32 {
	if p < 0 || p >= len(idx.parts) {
		return 0
	}
	return idx.parts[p].Offset
}

// PartitionCount returns the number of vectors in partition p.
func (idx *IVFIndex) PartitionVectorCount(p int) uint32 {
	if p < 0 || p >= len(idx.parts) {
		return 0
	}
	return idx.parts[p].Count
}

// LoadIndex mmaps the IVF binary file at path and parses its header
// and partition metadata. The returned IVFIndex holds the mmap'd region;
// callers must call Close to release it.
func LoadIndex(path string) (*IVFIndex, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat index: %w", err)
	}
	sz := fi.Size()
	if sz < ivfHeaderSize+ivfPartitionMetaSize {
		return nil, fmt.Errorf("index file too small (%d bytes)", sz)
	}

	data, err := syscall.Mmap(int(f.Fd()), 0, int(sz), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mmap index: %w", err)
	}

	idx := &IVFIndex{
		data: data,
		header: ivfHeader{
			Magic:      [4]byte{},
			Version:    0,
			Partitions: 0,
			NumVectors: 0,
			Dimensions: 0,
			Encoding:   0,
		},
		parts: nil,
	}
	if err := idx.parseHeader(); err != nil {
		return nil, errors.Join(err, syscall.Munmap(data))
	}
	if err := idx.parsePartitions(); err != nil {
		return nil, errors.Join(err, syscall.Munmap(data))
	}
	return idx, nil
}

// Close releases the mmap'd memory region.
func (idx *IVFIndex) Close() error {
	if idx.data == nil {
		return nil
	}
	err := syscall.Munmap(idx.data)
	idx.data = nil
	idx.parts = nil
	return err
}

// parseHeader reads and validates the binary header at the start of data.
func (idx *IVFIndex) parseHeader() error {
	if len(idx.data) < ivfHeaderSize {
		return fmt.Errorf("file too small for header (%d < %d)", len(idx.data), ivfHeaderSize)
	}
	h := &idx.header
	copy(h.Magic[:], idx.data[0:4])
	if string(h.Magic[:]) != ivfMagic {
		return fmt.Errorf("bad magic: got %q, want %q", string(h.Magic[:]), ivfMagic)
	}
	h.Version = binary.LittleEndian.Uint32(idx.data[4:8])
	h.Partitions = binary.LittleEndian.Uint32(idx.data[8:12])
	h.NumVectors = binary.LittleEndian.Uint32(idx.data[12:16])
	h.Dimensions = binary.LittleEndian.Uint32(idx.data[16:20])
	h.Encoding = binary.LittleEndian.Uint32(idx.data[20:24])

	if h.Version != 1 {
		return fmt.Errorf("unsupported version: %d", h.Version)
	}
	if h.Dimensions != ivfDimCount {
		return fmt.Errorf("unexpected dimensions: %d (want %d)", h.Dimensions, ivfDimCount)
	}
	return nil
}

// parsePartitions reads all partition metadata entries.
func (idx *IVFIndex) parsePartitions() error {
	n := int(idx.header.Partitions)
	metaStart := ivfHeaderSize
	metaEnd := metaStart + n*ivfPartitionMetaSize
	if len(idx.data) < metaEnd {
		return fmt.Errorf("file too small for partition metadata (%d < %d)", len(idx.data), metaEnd)
	}
	idx.parts = make([]ivfPartMeta, n)
	off := metaStart
	for i := 0; i < n; i++ {
		pm := &idx.parts[i]
		for j := 0; j < 14; j++ {
			pm.Centroid[j] = math.Float32frombits(binary.LittleEndian.Uint32(idx.data[off:]))
			off += 4
		}
		pm.Offset = binary.LittleEndian.Uint32(idx.data[off:])
		off += 4
		pm.Count = binary.LittleEndian.Uint32(idx.data[off:])
		off += 4
	}
	return nil
}

// dataStart returns the byte offset where the vector/label data section begins.
func (idx *IVFIndex) dataStart() int {
	return ivfHeaderSize + int(idx.header.Partitions)*ivfPartitionMetaSize
}

// VectorAt returns the quantized uint8 vector at position vecIdx within
// the data section. The caller is responsible for bounds checking.
func (idx *IVFIndex) VectorAt(vecIdx uint32) [14]uint8 {
	base := idx.dataStart() + int(vecIdx)*14
	var out [14]uint8
	copy(out[:], idx.data[base:base+14])
	return out
}

// LabelAt returns the label for the vector at position vecIdx.
// 0 = legit, 1 = fraud.
func (idx *IVFIndex) LabelAt(vecIdx uint32) uint8 {
	labelOffset := idx.dataStart() + int(idx.header.NumVectors)*14 + int(vecIdx)
	return idx.data[labelOffset]
}

// QuantizeVector converts a float64 query vector to uint8 using the same
// mapping used during index construction:
//
//	sentinel -1 → 255 (exclusive sentinel value)
//	value in [0,1] → uint8(value * 254) (maps 1.0 → 254)
//
// This ensures 255 is exclusively for sentinel values and never collides
// with valid normalized values. Direct integer distance comparison works
// because sentinel-sentinel pairs have distance 0 (255-255=0) while
// sentinel-normal pairs have large distance.
func QuantizeVector(v [14]float64) [14]uint8 {
	var out [14]uint8
	for i := 0; i < 14; i++ {
		if v[i] < 0 {
			out[i] = ivfSentinelQuantized
		} else {
			out[i] = uint8(math.Round(v[i] * 254))
		}
	}
	return out
}
