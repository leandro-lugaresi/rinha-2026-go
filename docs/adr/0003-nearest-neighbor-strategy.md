# ADR 0003: Nearest Neighbor Search Strategy

## Status

Accepted

## Context

The fraud detection task requires finding the 5 nearest vectors (by Euclidean distance) to an incoming transaction's 14-dimensional vector, among a reference set of 3,000,000 labeled vectors. The result determines the fraud score: `fraud_score = number_of_frauds_among_the_5 / 5`. A transaction is approved when `fraud_score < 0.6`.

Brute-force search over 3M vectors with 14 dimensions per query involves approximately 3,000,000 × 14 = 42,000,000 distance operations per query. At the latency targets required for a competitive p99 score (ideally sub-10ms), this is prohibitively expensive.

The challenge evaluation documentation explicitly recommends ANN approaches (HNSW, IVF, VP Trees) as alternatives to brute force. The reference index format (see ADR 0002) uses an IVF (Inverted File Index) structure with 8192 partitions, which enables partition pruning at query time.

## Decision

We use an IVF-based nearest neighbor search strategy with the following parameters and flow:

**Partition count**: 8192. Vectors in the reference set are assigned to partitions using k-means clustering (k=8192) during index construction. Each partition is identified by its centroid in 14-dimensional space.

**Initial probe**: On a query, the incoming 14-dimensional vector is compared against all 8192 centroids using Euclidean distance. The 32 nearest centroids (by distance) are selected as the initial probe set. This number balances recall (not missing too many true nearest neighbors) against the computational cost of scanning more partitions.

**Candidate scanning**: All vectors stored in the 32 probed partitions are collected as candidates. Each candidate is a 14-dimensional uint8 vector. The squared Euclidean distance is computed across all 14 dimensions using integer arithmetic. The top K=5 candidates with the smallest distances are retained.

**Full dimensional reranking**: The squared Euclidean distance is computed across all 14 dimensions for each candidate. There is no dimensionality reduction or component pruning at query time. This ensures the fraud score is based on true nearest neighbors in the full 14-dimensional space, consistent with how the reference dataset was labeled (the evaluation used brute-force k-NN with Euclidean distance on all 14 dimensions).

**Expansion guard**: If fewer than K=5 candidates are found in the initial 32 probed partitions (possible when partition sizes are uneven), the search expands to the next-closest unprobed partitions until at least K candidates are collected or all partitions have been probed.

**No test payload lookup**: Test payloads must not be used as reference lookups. This guardrail is enforced by design: the reference index contains only the 3M vectors from `references.json.gz`, not any test transaction vectors.

```
Query vector (14D)
  -> Compare against 8192 centroids (14D)          [32 nearest selected]
  -> Scan all vectors in those 32 partitions        [collect candidates]
  -> Compute squared Euclidean distance, all 14D   [rank by distance]
  -> Select top 5 nearest
  -> fraud_score = fraud_count_in_top5 / 5
```

## Consequences

- **Positive**: IVF with 8192 partitions and probe-32 provides a strong tradeoff between recall and speed. The 14-dimensional space is well-suited to clustering methods; vectors that are nearby in the full 14D space tend to cluster together in partition space, making partition pruning effective.
- **Positive**: The expansion guard ensures recall is not compromised when partition sizes are uneven or when the query vector falls in a sparse region of the centroid space.
- **Positive**: Computing distances over all 14 dimensions (no pruning) maintains fidelity with the evaluation's labeling methodology, which used brute-force Euclidean distance on all 14 dimensions. This avoids a systematic bias that could arise from dimension-specific weighting.
- **Positive**: The approach is deterministic and easy to reproduce and test, since there is no randomness in the search path (no random graph walks like HNSW, no random sampling).
- **Negative**: The initial centroid scan over 8192 × 14 = 114,688 distance operations is still significant. However, centroid vectors are stored as float32 (56 bytes each) and fit comfortably in L2/L3 cache, making this pass fast in practice. SIMD-accelerated distance routines (if available) further reduce this cost.
- **Negative**: IVF performance degrades if query vectors fall in regions of the space where partitions are unevenly populated (e.g., dense clusters with millions of vectors in a single partition). The k-means partitioning with k=8192 over 3M vectors aims to produce reasonably balanced partitions, but edge cases exist.
- **Negative**: The probe-32 parameter is a tuning choice. A higher probe count increases recall but scans more vectors. A lower probe count is faster but risks missing true nearest neighbors. If the strategy underperforms in evaluation, the probe count is the primary knob to adjust.

## Alternatives considered

**Brute-force search over all 3M vectors.** This is the baseline approach used to generate the evaluation labels and provides perfect recall. However, at 42M operations per query, it is too slow to achieve competitive p99 latency. The evaluation documentation explicitly acknowledges this.

**HNSW (Hierarchical Navigable Small World).** HNSW builds a multi-layer graph structure that enables fast approximate nearest neighbor search. It typically offers better recall-latency tradeoffs than IVF for high-dimensional data. However, HNSW requires significant extra memory for the graph edges (each vector stores connections to its nearest neighbors in the graph) and is more complex to implement correctly. The memory overhead would stress the per-instance budget (see ADR 0004).

**VP Tree (Vantage Point Tree).** VP Trees are exact search structures that recursively partition the space based on distances to selected vantage points. They avoid the memory overhead of graph-based methods and provide exact search. However, VP Trees can be less efficient than IVF for very large datasets with high dimensionality, and their query time is less predictable (depends on tree depth and branching factor).

**Reduce dimensionality before search (e.g., PCA or projection).** Reducing to fewer than 14 dimensions would speed up the distance calculation but would change the geometry of the search space relative to the evaluation's 14D brute-force labeling. This could systematically degrade detection quality by projecting fraud-relevant variance onto a lower-dimensional subspace that does not preserve the discriminative directions.

**Scan only a random subset of partitions.** Random scanning provides no guarantee of finding the true nearest neighbors and would lead to inconsistent fraud scores across requests. The centroid-based probe approach gives deterministic, geometry-informed prioritization.

**Use a different distance metric (e.g., cosine similarity, Manhattan distance).** The evaluation labels were generated using Euclidean distance, as stated in the evaluation documentation. Using a different metric would search a different geometry and would not be comparable to the evaluation's k-NN labels, likely degrading detection quality.