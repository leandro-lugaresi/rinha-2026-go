# ADR 0004: Memory Budget Strategy

## Status

Accepted

## Context

The Rinha de Backend 2026 challenge imposes a hard limit of 1 CPU core and 350 MB of total memory across all services in the `docker-compose.yml`. Our topology (see ADR 0001) allocates 0.10 CPU and 20 MB to HAProxy, leaving 0.90 CPU and 330 MB for the two Go API instances combined. With two instances, the per-instance budget is approximately 0.45 CPU and 165 MB, though the split is not guaranteed to be exactly even in Docker Compose resource limits.

Each API instance must hold the complete reference index in memory to serve fraud detection queries at low latency. The alternative of loading data from disk on every request would introduce disk I/O latency that would make sub-10ms p99 targets unachievable.

The reference index is pre-processed into a compact binary format (see ADR 0002) using uint8 quantization. This ADR calculates whether the index fits within the per-instance memory budget and identifies the headroom available for Go runtime overhead.

## Decision

We target a per-instance memory footprint of approximately 65 MB for the reference index, leaving over 90 MB of headroom within the 160 MB per-instance allocation. The breakdown is as follows:

**uint8 quantized vectors**: 3,000,000 vectors × 14 dimensions × 1 byte = 42,000,000 bytes ≈ 42 MB.

**Label array**: 3,000,000 bytes (one uint8 per vector, 0=legit, 1=fraud) ≈ 3 MB.

**Partition metadata**: 8,192 partitions × (56 bytes centroid + 4 bytes offset + 4 bytes count) = 8,192 × 64 = 524,288 bytes ≈ 0.5 MB.

**Index file total (mmap'd)**: approximately 45.5 MB. The file on disk is slightly larger due to padding and potential alignment, but mmap does not load the full file into physical RAM until pages are accessed. The OS page cache handles this transparently.

**Go runtime overhead**: The Go runtime (scheduler, garbage collector, memory management) typically consumes 10–20 MB in a modestly loaded service. We budget 15 MB for runtime overhead, which covers the goroutine stack space for request handlers, the heap for request/response objects, and GC metadata.

**Total estimated per-instance index + runtime**: approximately 60–65 MB, well within the 160 MB per-instance limit.

The binary index file is opened with `os.Open` and memory-mapped using ` syscall.Mmap`. The mmap region is read-only and is not managed by Go's heap allocator. Go's garbage collector does not scan mmap'd pages, so there is no GC overhead associated with the index data itself.

The Go process's actual heap allocation is limited to request-scoped objects (the parsed transaction, the response struct, temporary scratch buffers for distance calculations). These are small and short-lived, and the GC is designed to handle them efficiently.

## Consequences

- **Positive**: The memory budget is comfortable. With approximately 65 MB used for index and runtime, each API instance has roughly 95 MB of headroom within its 160 MB limit. This headroom accommodates request concurrency (goroutine stacks, temporary buffers), GC heap growth, and any unexpected memory usage spikes.
- **Positive**: Using mmap for the index means the index data does not count against Go's heap limit. If the Go heap grows beyond the mmap region, it is bounded by the container's memory cgroup limit (160 MB), not by the full 350 MB. This provides isolation between the index (managed by the OS) and the application heap (managed by Go's GC).
- **Positive**: The compact uint8 format means the full index fits in a single Docker layer or build stage, avoiding the need to ship separate data volumes or download the index at startup.
- **Negative**: The mmap approach means the binary index file must be bundled in the container image. At ~46 MB, the index adds meaningful size to the image. Multi-stage builds can minimize the final image size by copying only the binary index and the compiled Go binary.
- **Negative**: The memory estimate assumes no additional in-memory structures beyond the index, labels, and partition metadata. Adding caches, additional indices, or auxiliary data structures would consume the headroom and could push the instance toward its memory limit.
- **Negative**: If Docker Compose resource limits are applied as hard limits (not reservations), the Go runtime's internal memory management may not be aware of the container limit and could occasionally request memory beyond the cgroup limit, causing an OOM kill. It is important to configure Go's `GOMEMLIMIT` or equivalent environment variable to stay within the intended budget if memory pressure becomes an issue.

## Alternatives considered

**float32 instead of uint8.** Storing vectors as float32 would require 3,000,000 × 14 × 4 = 168 MB just for the vector data, plus labels and metadata. This would exceed the per-instance budget of 160 MB even before accounting for Go runtime overhead, leaving no headroom. float32 is not feasible without splitting the index across instances or adding a database, both of which add complexity and latency.

**float16 (half-precision).** This would halve the per-dimension size to 2 bytes, yielding ~84 MB for vectors plus overhead. This fits within the budget but requires software float16 arithmetic in Go (no native SIMD support for float16), adding latency to every distance calculation. uint8 integer arithmetic is faster and the quantization error is acceptable given the normalized input range.

**Store index on disk, load on demand.** This would reduce memory usage but introduces disk I/O latency. For a workload targeting sub-10ms p99, disk seeks for each query would be prohibitive. SSD storage could help, but the additional latency budget consumed by I/O would likely push p99 above competitive thresholds.

**Two-stage index: hot (in-memory) + cold (on-disk).** This approach keeps recently accessed partitions in memory and spills others to disk. It adds complexity (tracking access patterns, managing eviction) for marginal benefit. The compact uint8 index already fits in memory, making this optimization unnecessary.

**Memory-mapped file with compression (e.g., zstd).** Compressing the on-disk index would reduce image size but would require decompression at runtime, adding CPU overhead per query. The current balance favors CPU efficiency over disk savings, since the index already fits within the budget without compression.

**Split index across both API instances (horizontal partitioning).** Rather than each instance holding the full index, partition the 3M vectors between the two instances. This would halve the per-instance memory footprint to ~22.5 MB. However, it would introduce cross-instance communication for queries whose nearest vectors reside in the other instance's partition, adding network latency and complexity. Given that the full index already fits in memory, the simplicity of a full-index-per-instance approach is preferred.

**Store labels in a separate compact structure (e.g., bitmap).** A bitmap for 3M labels would be 3,000,000 bits ≈ 0.375 MB, which is smaller than the 3 MB byte array. However, the byte array provides simpler random access (label for vector at offset O is at byte O) and the difference (2.6 MB) is not meaningful against the overall budget. The byte array is retained for clarity and access simplicity.