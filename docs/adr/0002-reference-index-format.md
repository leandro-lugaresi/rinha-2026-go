# ADR 0002: Reference Index Format

## Status

Accepted

## Context

The reference dataset consists of 3,000,000 labeled 14-dimensional vectors stored in `references.json.gz` (gzip-compressed, ~16 MB on disk, ~284 MB uncompressed). Each vector's values are normalized floats in the range `[0.0, 1.0]`, with the exception of dimensions 5 (`minutes_since_last_tx`) and 6 (`km_from_last_tx`), which use `-1` as a sentinel to represent "no previous transaction." The dataset is static; it does not change between requests or during the test. A companion file `mcc_risk.json` maps merchant category codes to risk scores, and `normalization.json` provides constants used in the normalization formula.

The Rinha de Backend 2026 scoring rewards low p99 latency (targeting sub-millisecond to low-single-digit milliseconds per query) and correct fraud detection. Loading and decompressing `references.json.gz` at runtime for every query is not acceptable. The reference data must be pre-processed into a format optimized for fast k-nearest-neighbor search.

The challenge's evaluation page explicitly recommends ANN approaches (HNSW, IVF) or VP Trees as alternatives to brute-force search over 3M vectors. Our approach uses IVF (Inverted File Index), which groups vectors by their nearest centroid and allows queries to narrow the search space by probing only relevant partitions.

## Decision

We pre-process the reference dataset offline (at container build time or startup) into a compact binary IVF index file with the following structure:

```
Header (24 bytes)
  - Magic number / version:    4 bytes  (e.g., "RVI1")
  - Version:                   4 bytes  (uint32, currently 1)
  - Number of partitions:       4 bytes  (uint32, 8192)
  - Number of vectors:          4 bytes  (uint32, 3_000_000)
  - Dimensions:                4 bytes  (uint32, 14)
  - Encoding:                  4 bytes  (uint32, 1 = uint8 quantized)

Partition metadata (per partition, 8192 entries)
  - Centroid:                  14 × 4 = 56 bytes  (float32, 14 dimensions)
  - Offset:                    4 bytes  (uint32, byte offset into the data section where this partition's vectors begin)
  - Count:                     4 bytes  (uint32, number of vectors in this partition)

Data section (row-major)
  - Quantized vectors:         3_000_000 × 14 bytes  (uint8, one byte per dimension)
  - Labels:                    3_000_000 × 1 byte    (uint8, 0 = legit, 1 = fraud)

Footer (optional padding for alignment)
```

**uint8 quantization**: each float32 value in `[0.0, 1.0]` is linearly mapped to a `uint8` in `[0, 255]`. The sentinel value `-1` for dimensions 5 and 6 is encoded as `255` (outside the normal `[0, 254]` range), preserving the distinction between "no history" and any valid normalized value. The quantization uses a deterministic scale factor so that query vectors can be quantized using the same mapping and compared with the stored uint8 values using integer distance metrics.

**Sentinel encoding for dimensions 5 and 6**: because the sentinel value `-1` represents a fundamentally different semantic state (no previous transaction) rather than a numeric magnitude, it is encoded as `255` in the uint8 representation. This ensures that the squared Euclidean distance between two "no history" transactions on these dimensions is zero (255 - 255 = 0), while the distance between a "no history" and any real value is large (e.g., 255 - 128 = 127, squared = 16129). This naturally clusters "no history" transactions together in the vector space, which is the intended behavior described in the dataset documentation.

The binary file is mmap'd (memory-mapped) at runtime, which allows the operating system to handle paging and provides fast random access without requiring the entire file to be copied into the Go heap.

## Consequences

- **Positive**: The binary format reduces the on-disk and in-memory footprint dramatically compared to the gzipped JSON. uint8 quantization reduces each dimension from 4 bytes (float32) to 1 byte, an 4× reduction before accounting for any compression benefit from the binary layout.
- **Positive**: mmap provides O(1) random access to any byte offset in the file without Go heap allocation overhead. The OS manages paging, and for locality-of-reference scenarios (probing nearby partitions), hot data stays in page cache.
- **Positive**: The IVF structure enables partition pruning at query time. Rather than scanning all 3M vectors, the query only needs to examine vectors in the most relevant partitions (see ADR 0003).
- **Positive**: The sentinel encoding preserves the semantic distinction of "no history" transactions without requiring special-case handling in the search loop.
- **Negative**: uint8 quantization introduces a small rounding error compared to float32 precision. However, the normalized input values are already in `[0.0, 1.0]`, so the quantization step from 255 discrete levels provides sufficient granularity for the distance comparisons used in KNN.
- **Negative**: Building the index requires an offline preprocessing step. If the preprocessing is done at container startup, it adds to the container's initialization time. This should be done once at image build time where possible.
- **Negative**: The mmap approach means the binary index file must be bundled in the container image or mounted as a volume. The file size (~65 MB, see ADR 0004) is manageable for a container image.

## Alternatives considered

**Keep data as gzipped JSON and decompress at startup into a float32 slice.** This would avoid custom binary format complexity but would use approximately 284 MB for the uncompressed data in float32 (3M × 14 × 4 bytes), plus the overhead of Go slice metadata. This exceeds the per-instance memory budget and provides no search locality benefits.

**Use float16 (float32) instead of uint8 quantization.** float16 would halve the per-dimension size from 4 bytes to 2 bytes, still preserving more precision than uint8. However, float16 arithmetic is not natively supported by standard Go, requiring software emulation that adds latency. uint8 integer arithmetic is fast and sufficient given the normalized `[0.0, 1.0]` input range.

**Use HNSW instead of IVF.** HNSW (Hierarchical Navigable Small World) is a graph-based ANN method that typically offers better recall-latency tradeoffs than IVF for high-dimensional data. However, HNSW has significant memory overhead for the graph structure (edges between nodes) and is more complex to implement correctly. IVF with k-means partitioning provides a simpler, predictable memory footprint and is well-suited to the 14-dimensional case, where clustering methods work well.

**Store vectors in a key-value store (e.g., badger, boltdb) keyed by partition ID.** This would add external library dependencies and query latency due to storage engine overhead. A flat binary file with mmap provides simpler, lower-latency access with no additional dependencies.

**Use variable-length encoding or compression (e.g., zstd) on the binary data.** Compression would reduce the file size but would require decompression at query time, adding latency that is unacceptable for the sub-10ms p99 target. The current uint8 representation is already compact enough.

**Store labels separately from vectors (e.g., in a distinct section or external file).** Keeping labels in the same binary file but in a separate section simplifies the file layout and allows the labels array to be read in a single sequential pass when scoring candidate vectors. This is the approach chosen.