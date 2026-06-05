# Draft: Rinha 2026 Go Backend

## Requirements (confirmed)
- "Implement a Go backend for Rinha de Backend 2026 fraud detection."
- "Do not jump straight into coding. Work through the design collaboratively."
- "As decisions emerge, create documentation and ADRs."
- API must expose `GET /ready` and `POST /fraud-score` on port `9999`.
- Deployment must include at least one load balancer and two API instances with round-robin distribution.
- Total compose resource limits must stay within `1 CPU` and `350MB` memory, use bridge networking, and avoid host/privileged modes.
- Fraud scoring must vectorize each transaction into the documented 14 dimensions, preserve `-1` sentinels for `last_transaction: null`, use MCC risk/defaults and normalization constants from resources, find 5 nearest reference vectors, compute `fraud_score`, and approve only when `fraud_score < 0.6`.
- Final reranking must use all 14 dimensions if bucketed routing is selected, unless an ADR explicitly justifies otherwise.

## Technical Decisions
- Runtime topology: two Go API instances, each owning a compact in-process reference index, behind HAProxy as pure round-robin load balancer.
- HAProxy responsibility boundary: forward requests only; no fraud detection, payload inspection, or conditional response logic.
- [pending] Reference preprocessing/index format under memory budget.
- Nearest-neighbor strategy for first implementation: IVF-style index, with candidate generation from partitions and final reranking over all 14 dimensions.
- Index artifact format: offline-built compact binary IVF file loaded by each API instance.
- Vector encoding: `uint8` quantization, with sentinel handling preserving `-1` semantics for dimensions 5 and 6.
- Initial IVF scale: 8192 partitions before measurement tuning.
- Accuracy policy: conservative candidate expansion, tuned by benchmark/load-test evidence rather than aggressive latency-first shortcuts.
- HTTP runtime: idiomatic `net/http` with allocation-conscious handlers.
- Initial IVF query policy: probe 32 nearest partitions before tuning.
- Initial compose resource split: each API `0.45 CPU / 160MB`; HAProxy `0.10 CPU / 20MB`, totaling `1 CPU / 340MB`.
- Test strategy: TDD for core fraud logic before implementation, especially vectorization, `last_transaction: null`, unknown merchant/MCC defaults, score threshold, and API shape.
- [pending] Measurement strategy for memory, latency, and detection quality.

## Research Findings
- `README.md` / `API.md`: required endpoints and response shape are fixed.
- `DETECTION_RULES.md`: 14-dimensional vector order and normalization formulas are fixed; Euclidean brute force labels the provided test set, but algorithm choice is open.
- `DATASET.md`: reference dataset is 3M labeled vectors, ~16MB gzipped / ~284MB uncompressed; `mcc_risk.json` and `normalization.json` are tiny and stable.
- `ARCHITECTURE.md`: docker compose must expose the load balancer on port 9999, with public linux-amd64 images and aggregate resource limits.
- `EVALUATION.md`: low HTTP errors and <=15% failure rate are critical; p99 latency improves score logarithmically down to 1ms.
- Repository exploration: project is greenfield; no Go files, `go.mod`, Dockerfile, compose, LB config, Makefile, CI, `info.json`, docs, or ADRs exist yet.
- Test exploration: no Go tests/benchmarks exist; official local harness lives in `.context/rinha-de-backend-2026/test/` with k6 smoke/load scripts and labeled test data, but project tests must be created from scratch.

## Open Questions
- Centroid/build method and recall-validation benchmark details should be specified in the work plan and ADR tasks.

## Scope Boundaries
- INCLUDE: Go API, vectorization, reference index/preprocessing, scoring, tests, benchmarks, docs/ADRs, docker-compose/local verification.
- EXCLUDE: Using test payloads as lookup/reference data, fraud logic in load balancer, undocumented complex index choices, blind optimization without measurement.
