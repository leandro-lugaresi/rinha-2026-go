# Rinha 2026 Go Fraud Backend

## TL;DR
> **Summary**: Build a greenfield Go fraud-detection backend for Rinha de Backend 2026, with ADRs first, HAProxy + two API instances, and compact in-process IVF indexes per API.
> **Deliverables**: Go API, vectorization/scoring tests, offline IVF index builder, benchmark commands/binaries, HAProxy config, Dockerfile, docker-compose, docs/ADRs, local HTTP/k6 verification.
> **Effort**: Large
> **Parallel**: YES - 4 waves
> **Critical Path**: ADRs → vectorization TDD → compact IVF builder/loader → API integration → compose/load verification

## Context

### Original Request
Implement a Go backend for Rinha de Backend 2026 fraud detection. Do not jump straight into coding; discuss architecture, create docs and ADRs as decisions emerge, then implement and keep docs aligned.

### Interview Summary
- Runtime topology: HAProxy load balancer + two Go API instances.
- HAProxy must be pure round-robin forwarding only; no fraud logic or payload inspection.
- Each API instance owns a compact in-process index.
- Search strategy: offline-built compact binary IVF file, `uint8` quantized vectors, 8192 partitions, initial query probes 32 partitions, conservative expansion, all-14-dimension final reranking.
- HTTP stack: Go stdlib `net/http`.
- Resource split: API containers `0.45 CPU / 160MB` each; HAProxy `0.10 CPU / 20MB`; total `1 CPU / 340MB`.
- Test strategy: TDD for core fraud logic.

### Metis Review (gaps addressed)
- Add exact/brute-force implementation as a correctness oracle and benchmark path, not as production runtime.
- Validate vectorization against the two worked examples in `DETECTION_RULES.md`.
- Guard Go weekday remapping: spec requires Monday=0, Sunday=6; Go returns Sunday=0.
- Define behavior for `customer.avg_amount <= 0`: avoid HTTP errors; clamp `amount_vs_avg` to `1.0` as a fail-safe high-risk ratio.
- Avoid HTTP 500 where possible; return a valid default decision if a request would otherwise fail.
- Prebuild the binary index outside request path; `/ready` must only become 2xx after index load.
- Module name correction from user: the full module path should be `github.com/leandro-lugaresi/rinha-2026-go`; alternatively, use simple internal module name `rinha-2026-go`. The previously scaffolded `github.com/leandro/rinha-2026-go` is wrong and must be corrected before further implementation.

## Work Objectives

### Core Objective
Return compliant fraud decisions by vectorizing requests, searching the compact IVF reference index, computing the fixed K=5 fraud score, and serving through HAProxy on port 9999.

### Deliverables
- `go.mod`, Go source under `cmd/` and `internal/`.
- `docs/adr/0001-runtime-topology.md` through `0004-memory-budget-strategy.md`.
- Offline index-builder command and compact binary index format.
- Unit tests, integration tests, benchmarks, and local HTTP verification scripts/commands.
- `Dockerfile`, `docker-compose.yml`, and HAProxy config.
- `info.json` submission metadata placeholder if absent.

### Definition of Done (verifiable conditions with commands)
- `go test ./...` passes.
- `go test -bench=. -benchmem ./...` runs benchmark coverage for vectorization, index search, and HTTP handler.
- `docker compose up --build` starts HAProxy and two API instances within resource limits.
- `curl -fsS http://localhost:9999/ready` returns 2xx only after both APIs are ready.
- `curl -fsS -X POST http://localhost:9999/fraud-score ...` returns JSON with `approved` boolean and numeric `fraud_score`.
- `.context/rinha-de-backend-2026/test/smoke.js` can run against `localhost:9999` and pass response-shape/status checks.

### Must Have
- Use official docs/resources from `.context/rinha-de-backend-2026`.
- Vector dimensions exactly follow `.context/rinha-de-backend-2026/docs/en/DETECTION_RULES.md:50-65`.
- Preserve `-1` semantics for `last_transaction: null` per `.context/rinha-de-backend-2026/docs/en/DATASET.md:23-26`.
- Use MCC default `0.5` per `.context/rinha-de-backend-2026/docs/en/DATASET.md:30-52`.
- Final neighbor reranking uses all 14 dimensions.
- No test payloads as reference/lookup data.

### Must NOT Have
- No fraud logic in HAProxy.
- No `host` or `privileged` mode.
- No runtime dependency on private images or ARM64-only images.
- No float64 parsed structs for the full 3M dataset at API runtime.
- No undocumented architectural shortcuts.

## Verification Strategy
> ZERO HUMAN INTERVENTION - all verification is agent-executed.
- Test decision: TDD core logic with Go `testing`.
- QA policy: Every task has agent-executed scenarios.
- Evidence: `.sisyphus/evidence/task-{N}-{slug}.{ext}`

## Execution Strategy

### Parallel Execution Waves
> Target: 5-8 tasks per wave. <3 per wave (except final) = under-splitting.
> Extract shared dependencies as Wave-1 tasks for max parallelism.

Wave 1: documentation/ADRs and Go scaffold only.
Wave 2: vectorization implementation, exact oracle, compact index format/builder, HTTP contract tests.
Wave 3: IVF search integration, API handlers, benchmarks, Docker/HAProxy compose.
Wave 4: memory/latency tuning, smoke/load verification, submission metadata/docs alignment.

### Dependency Matrix (full, all tasks)
- T1 blocks T3-T10 by establishing ADR decisions before implementation.
- T2 blocks all Go implementation tasks.
- T3 blocks T6, T7, T8.
- T4 blocks T6 and T8.
- T5 blocks T7 and T10.
- T6 blocks T8.
- T7 blocks T8 and T10.
- T8 blocks T9 and T10.
- T9 blocks T10.
- T10 blocks final verification.

### Agent Dispatch Summary (wave → task count → categories)
- Wave 1: 2 tasks — writing, quick.
- Wave 2: 4 tasks — deep, golang-oriented unspecified-high.
- Wave 3: 3 tasks — deep, devops-engineer, unspecified-high.
- Wave 4: 2 tasks — extreme optimization, verification.

## TODOs
> Implementation + Test = ONE task. Never separate.
> EVERY task MUST have: Agent Profile + Parallelization + QA Scenarios.

- [x] 1. Create docs/ADR foundation

  **What to do**: Create `docs/adr/` and write four ADRs: runtime topology, reference index format, nearest-neighbor strategy, memory budget. Use standard sections: Status, Context, Decision, Consequences, Alternatives considered. Mark all as `Accepted` because decisions were confirmed in planning.
  **Must NOT do**: Do not implement code in this task. Do not leave IVF or memory choices only in comments/chat.

  **Recommended Agent Profile**:
  - Category: `writing` - Reason: documentation and ADR authoring.
  - Skills: [`writing-clearly-and-concisely`] - concise, high-signal ADRs.
  - Omitted: [`golang-pro`] - no Go code in this task.

  **Parallelization**: Can Parallel: YES | Wave 1 | Blocks: T3-T10 | Blocked By: none

  **References**:
  - Pattern: `.context/rinha-de-backend-2026/docs/en/ARCHITECTURE.md:3-43` - topology/resource constraints.
  - Pattern: `.context/rinha-de-backend-2026/docs/en/DATASET.md:12-26` - reference file and sentinel behavior.
  - Pattern: `.context/rinha-de-backend-2026/docs/en/EVALUATION.md:172-188` - measurement and ANN guidance.

  **Acceptance Criteria**:
  - [ ] `test -f docs/adr/0001-runtime-topology.md`
  - [ ] `test -f docs/adr/0002-reference-index-format.md`
  - [ ] `test -f docs/adr/0003-nearest-neighbor-strategy.md`
  - [ ] `test -f docs/adr/0004-memory-budget-strategy.md`
  - [ ] Each ADR contains `Status`, `Context`, `Decision`, `Consequences`, and `Alternatives considered`.

  **QA Scenarios**:
  ```
  Scenario: ADR structure exists
    Tool: Bash
    Steps: grep -R "^## Status\|^## Context\|^## Decision\|^## Consequences\|^## Alternatives considered" docs/adr
    Expected: All four ADRs include all five headings.
    Evidence: .sisyphus/evidence/task-1-adrs.txt

  Scenario: ADRs reject non-goals
    Tool: Bash
    Steps: grep -R "test payloads\|load balancer.*logic\|host.*privileged" docs/adr
    Expected: Guardrails are explicitly documented.
    Evidence: .sisyphus/evidence/task-1-adrs-guardrails.txt
  ```

  **Commit**: YES | Message: `docs: add architecture decision records` | Files: `docs/adr/*`

- [x] 2. Initialize Go project and source layout

  **What to do**: Create `go.mod`, `.gitignore`, and layout: `cmd/api`, `cmd/indexer`, `internal/api`, `internal/vectorize`, `internal/reference`, `internal/search`, `internal/scoring`, `internal/resources`. Add `Makefile` targets: `test`, `bench`, `build`, `run`, `docker-build`, `smoke`.
  **Correction required before continuing**: Change module name from `github.com/leandro/rinha-2026-go` to `github.com/leandro-lugaresi/rinha-2026-go` (preferred full path) or `rinha-2026-go` (acceptable simple internal name), and update all imports accordingly.
  **Must NOT do**: Do not add third-party framework dependencies unless needed by later tasks.

  **Recommended Agent Profile**:
  - Category: `quick` - Reason: greenfield scaffold with clear paths.
  - Skills: [`golang-pro`] - idiomatic Go layout.
  - Omitted: [`devops-engineer`] - Docker comes later.

  **Parallelization**: Can Parallel: YES | Wave 1 | Blocks: T3-T10 | Blocked By: none

  **References**:
  - Pattern: Repository exploration found no Go files/go.mod; create from scratch.
  - External: Go project layout conventions; keep `internal/` for non-public packages.

  **Acceptance Criteria**:
  - [ ] `go test ./...` exits 0 with scaffold packages.
  - [ ] `make test` runs `go test ./...`.
  - [ ] `make bench` runs `go test -bench=. -benchmem ./...`.

  **QA Scenarios**:
  ```
  Scenario: Fresh Go scaffold compiles
    Tool: Bash
    Steps: go test ./...
    Expected: Exit code 0.
    Evidence: .sisyphus/evidence/task-2-go-test.txt

  Scenario: Make targets exist
    Tool: Bash
    Steps: make test && make bench
    Expected: Both commands execute without missing-target errors.
    Evidence: .sisyphus/evidence/task-2-make.txt
  ```

  **Commit**: YES | Message: `chore: initialize go project scaffold` | Files: `go.mod`, `.gitignore`, `Makefile`, `cmd/**`, `internal/**`

- [x] 3. TDD vectorization and scoring rules

  **What to do**: Write tests first, then implement vectorization and scoring. Cover the two worked examples, clamp behavior, Go weekday remapping Monday=0/Sunday=6, `last_transaction: null`, unknown merchant, unknown MCC default, duplicate known merchants, amount-vs-average zero guard, and threshold behavior.
  **Must NOT do**: Do not use test payloads as reference/lookup data.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` - Reason: correctness-heavy implementation.
  - Skills: [`golang-pro`, `tdd`, `test-antipatterns`] - table tests and core logic.
  - Omitted: [`extreme-software-optimization`] - no optimization before correctness.

  **Parallelization**: Can Parallel: YES | Wave 2 | Blocks: T6, T8 | Blocked By: T1, T2

  **References**:
  - Pattern: `.context/rinha-de-backend-2026/docs/en/DETECTION_RULES.md:50-65` - 14 dimensions and formulas.
  - Pattern: `.context/rinha-de-backend-2026/docs/en/DETECTION_RULES.md:12-41` - legit worked example.
  - Pattern: `.context/rinha-de-backend-2026/docs/en/DETECTION_RULES.md:110-139` - fraud worked example.
  - API/Type: `.context/rinha-de-backend-2026/docs/en/API.md:15-75` - request/response schema.

  **Acceptance Criteria**:
  - [ ] `go test -run 'TestVectorization' ./internal/vectorize` passes.
  - [ ] `go test -run 'TestScoreThreshold' ./internal/scoring` passes.
  - [ ] Worked example vectors match documented values within `±0.0001`.
  - [ ] Unknown MCC returns `0.5`; `last_transaction: null` returns `-1` for dims 5 and 6.

  **QA Scenarios**:
  ```
  Scenario: Worked examples match docs
    Tool: Bash
    Steps: go test -run 'TestVectorization_(LegitExample|FraudExample)' ./internal/vectorize
    Expected: Both examples pass with tolerance ±0.0001.
    Evidence: .sisyphus/evidence/task-3-vector-examples.txt

  Scenario: Defensive edge cases avoid HTTP-error roots
    Tool: Bash
    Steps: go test -run 'Test(VectorizationNullLastTransaction|MCCDefault|DayOfWeek|AmountVsAvgZero)' ./internal/vectorize
    Expected: All edge-case tests pass.
    Evidence: .sisyphus/evidence/task-3-vector-edge.txt
  ```

  **Commit**: YES | Message: `feat: add fraud vectorization rules` | Files: `internal/vectorize/**`, `internal/scoring/**`

- [x] 4. Add exact KNN oracle and fixtures

  **What to do**: Implement an exact Euclidean KNN search over small fixture references (`example-references.json`) for tests and benchmark validation. This is not the production runtime path. Use squared L2 for ordering; no `sqrt` needed.
  **Must NOT do**: Do not scan full `references.json.gz` in API request path.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` - Reason: search correctness baseline.
  - Skills: [`golang-pro`, `tdd`] - deterministic tests.
  - Omitted: [`devops-engineer`] - no containers.

  **Parallelization**: Can Parallel: YES | Wave 2 | Blocks: T6, T7, T10 | Blocked By: T1, T2

  **References**:
  - Pattern: `.context/rinha-de-backend-2026/docs/en/DATASET.md:12-26` - reference format and labels.
  - Pattern: `.context/rinha-de-backend-2026/docs/en/VECTOR_SEARCH.md:74-117` - Euclidean KNN concept.

  **Acceptance Criteria**:
  - [ ] `go test -run 'TestExactKNN' ./internal/search` passes.
  - [ ] Exact oracle returns K=5 sorted nearest labels for fixture data.
  - [ ] Distance ordering uses all 14 dimensions.

  **QA Scenarios**:
  ```
  Scenario: Exact oracle uses all dimensions
    Tool: Bash
    Steps: go test -run TestExactKNNUsesAllDimensions ./internal/search
    Expected: Test fails if any dimension is ignored.
    Evidence: .sisyphus/evidence/task-4-exact-all-dims.txt

  Scenario: Fixture reference parsing works
    Tool: Bash
    Steps: go test -run TestExampleReferencesLoad ./internal/reference
    Expected: example-references.json loads vectors and labels.
    Evidence: .sisyphus/evidence/task-4-fixtures.txt
  ```

  **Commit**: YES | Message: `test: add exact knn oracle` | Files: `internal/search/**`, `internal/reference/**`

- [x] 5. Build compact binary IVF indexer

  **What to do**: Implement `cmd/indexer` to read `.context/rinha-de-backend-2026/resources/references.json.gz`, create 8192 IVF partitions, encode vectors as `uint8`, store labels compactly, write a binary file with header/version, centroids, partition offsets, quantized vectors, and labels. Document sentinel encoding in ADR 0002/0003. Use deterministic sampling/seed for centroid build.
  **Must NOT do**: Do not parse full references into `float64` structs retained in memory. Do not depend on `test/test-data.json`.

  **Recommended Agent Profile**:
  - Category: `deep` - Reason: index format, streaming parse, memory constraints.
  - Skills: [`golang-pro`, `extreme-software-optimization`] - memory-aware builder and measurement.
  - Omitted: [`devops-engineer`] - Docker integration later.

  **Parallelization**: Can Parallel: NO | Wave 2 | Blocks: T7, T8, T10 | Blocked By: T1, T2, T4

  **References**:
  - Pattern: `.context/rinha-de-backend-2026/docs/en/DATASET.md:5-9` - resource files.
  - Pattern: `.context/rinha-de-backend-2026/docs/en/DATASET.md:81` - files can be preprocessed.
  - Pattern: `docs/adr/0002-reference-index-format.md` - format contract to create/update.

  **Acceptance Criteria**:
  - [ ] `go run ./cmd/indexer -input .context/rinha-de-backend-2026/resources/references.json.gz -output ./var/references.ivf` completes.
  - [ ] `go test -run 'TestIVFFileFormat' ./internal/reference` passes.
  - [ ] Binary index header records version, vector dimensions=14, partitions=8192, encoding=uint8.
  - [ ] Builder prints peak memory and output file size.

  **QA Scenarios**:
  ```
  Scenario: Binary IVF index is generated
    Tool: Bash
    Steps: mkdir -p var && go run ./cmd/indexer -input .context/rinha-de-backend-2026/resources/references.json.gz -output var/references.ivf
    Expected: var/references.ivf exists and indexer exits 0.
    Evidence: .sisyphus/evidence/task-5-index-build.txt

  Scenario: Sentinel encoding round-trips
    Tool: Bash
    Steps: go test -run TestQuantizedSentinelRoundTrip ./internal/reference
    Expected: Dimensions 5/6 preserve no-history semantics through uint8 encoding.
    Evidence: .sisyphus/evidence/task-5-sentinel.txt
  ```

  **Commit**: YES | Message: `feat: add compact ivf index builder` | Files: `cmd/indexer/**`, `internal/reference/**`, `docs/adr/*`

- [x] 6. Implement IVF searcher and recall benchmark

  **What to do**: Load the binary IVF index in-process, probe 32 nearest centroids, scan every candidate in those partitions with no lower candidate cap, rerank using all 14 quantized dimensions, select K=5, and compute fraud count. If the 32 probed partitions yield fewer than K candidates because of empty partitions/corrupt index, continue probing nearest remaining partitions until K is available or the index is exhausted. Add benchmarks and a recall comparison against the exact oracle on sampled/example data. Make probe count configurable by env with default `32`.
  **Must NOT do**: Do not rerank only routing dimensions. Do not hardcode test payload labels.

  **Recommended Agent Profile**:
  - Category: `deep` - Reason: ANN correctness/performance balance.
  - Skills: [`golang-pro`, `extreme-software-optimization`] - benchmark-driven search.
  - Omitted: [`devops-engineer`] - runtime only.

  **Parallelization**: Can Parallel: NO | Wave 3 | Blocks: T8, T10 | Blocked By: T3, T4, T5

  **References**:
  - Pattern: `.context/rinha-de-backend-2026/docs/en/DETECTION_RULES.md:93-101` - K=5 and threshold.
  - Pattern: `docs/adr/0003-nearest-neighbor-strategy.md` - IVF decision.

  **Acceptance Criteria**:
  - [ ] `go test -run 'TestIVF' ./internal/search` passes.
  - [ ] `go test -bench 'BenchmarkIVFSearch' -benchmem ./internal/search` reports allocations and latency.
  - [ ] Searcher defaults to probing 32 partitions.
  - [ ] Searcher scans all candidates from probed partitions; no hidden candidate truncation before all-14-dimension rerank.
  - [ ] If fewer than K candidates are found, searcher expands beyond 32 partitions until K candidates are available or index is exhausted.
  - [ ] Tests fail if reranking ignores any of 14 dimensions.

  **QA Scenarios**:
  ```
  Scenario: IVF search returns K=5 fraud score
    Tool: Bash
    Steps: go test -run TestIVFSearchScoresKFive ./internal/search
    Expected: Result fraud_score is one of 0.0, 0.2, 0.4, 0.6, 0.8, 1.0.
    Evidence: .sisyphus/evidence/task-6-ivf-score.txt

  Scenario: IVF benchmark captures evidence
    Tool: Bash
    Steps: go test -bench BenchmarkIVFSearch -benchmem ./internal/search
    Expected: Benchmark output includes ns/op and allocs/op.
    Evidence: .sisyphus/evidence/task-6-ivf-bench.txt

  Scenario: Conservative expansion handles sparse partitions
    Tool: Bash
    Steps: go test -run TestIVFExpandsWhenCandidatesBelowK ./internal/search
    Expected: Search probes beyond the default when sparse/empty partitions yield fewer than K candidates.
    Evidence: .sisyphus/evidence/task-6-ivf-expansion.txt
  ```

  **Commit**: YES | Message: `feat: add ivf fraud search` | Files: `internal/search/**`, `internal/reference/**`

- [x] 7. Implement net/http API server

  **What to do**: Implement `cmd/api` with `/ready` and `/fraud-score`. Load resources/index on startup; `/ready` returns 2xx only when index is ready. Parse request JSON, vectorize, search, score, and respond. On malformed/exceptional requests, avoid 500 where possible by returning HTTP 200 default `{"approved":true,"fraud_score":0.0}` only after logging/metric hook.
  **Must NOT do**: Do not put business logic outside API. Do not return non-JSON success responses.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` - Reason: API integration with core logic.
  - Skills: [`golang-pro`] - idiomatic `net/http`.
  - Omitted: [`hono`, `elysia`] - irrelevant stack.

  **Parallelization**: Can Parallel: NO | Wave 3 | Blocks: T9, T10 | Blocked By: T3, T6

  **References**:
  - API/Type: `.context/rinha-de-backend-2026/docs/en/API.md:6-75` - endpoints and payload shape.
  - Pattern: `.context/rinha-de-backend-2026/docs/en/EVALUATION.md:181-183` - avoid HTTP errors.

  **Acceptance Criteria**:
  - [ ] `go test -run 'TestAPI' ./internal/api` passes.
  - [ ] `go run ./cmd/api` serves `GET /ready` and `POST /fraud-score`.
  - [ ] Response JSON contains exactly usable `approved` bool and `fraud_score` number.

  **QA Scenarios**:
  ```
  Scenario: Ready endpoint works after load
    Tool: Bash
    Steps: sh -c 'RINHA_INDEX_PATH=var/references.ivf PORT=9999 go run ./cmd/api >.sisyphus/evidence/task-7-api.log 2>&1 & pid=$!; sleep 2; curl -fsS http://localhost:9999/ready; status=$?; kill $pid; wait $pid || true; exit $status'
    Expected: 2xx response after index/resource readiness.
    Evidence: .sisyphus/evidence/task-7-ready.txt

  Scenario: Fraud endpoint returns contract shape
    Tool: Bash
    Steps: sh -c 'RINHA_INDEX_PATH=var/references.ivf PORT=9999 go run ./cmd/api >.sisyphus/evidence/task-7-api.log 2>&1 & pid=$!; sleep 2; python3 -c '\''import json; print(json.dumps(json.load(open(".context/rinha-de-backend-2026/resources/example-payloads.json"))[0]))'\'' | curl -fsS -X POST http://localhost:9999/fraud-score -H "Content-Type: application/json" --data-binary @-; status=$?; kill $pid; wait $pid || true; exit $status'
    Expected: For an individual payload fixture, JSON includes approved and fraud_score with HTTP 200.
    Evidence: .sisyphus/evidence/task-7-fraud-http.json
  ```

  **Commit**: YES | Message: `feat: add fraud scoring api` | Files: `cmd/api/**`, `internal/api/**`

- [x] 8. Add Dockerfile, HAProxy, and compose topology

  **What to do**: Create multi-stage linux-amd64-compatible Dockerfile, HAProxy config with round-robin to two API services and `/ready` health checks, and root `docker-compose.yml` using bridge networking. Apply resource limits: api01/api02 each `0.45` CPU and `160MB`; haproxy `0.10` CPU and `20MB`. Expose port 9999 on HAProxy.
  **Must NOT do**: Do not use host network, privileged mode, or HAProxy request-body logic.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` - Reason: deployment wiring.
  - Skills: [`devops-engineer`] - Docker and HAProxy config.
  - Omitted: [`cloudflare`] - unrelated.

  **Parallelization**: Can Parallel: YES | Wave 3 | Blocks: T10 | Blocked By: T7

  **References**:
  - Pattern: `.context/rinha-de-backend-2026/docs/en/ARCHITECTURE.md:3-43` - topology and constraints.
  - Pattern: `docs/adr/0001-runtime-topology.md` - accepted HAProxy decision.

  **Acceptance Criteria**:
  - [ ] `docker compose config` succeeds.
  - [ ] Compose has one HAProxy service and two API services.
  - [ ] Total configured limits are `<= 1 CPU` and `<= 350MB`.
  - [ ] HAProxy listens on host port 9999.
  - [ ] Both API containers receive `RINHA_INDEX_PATH` and have the prebuilt `references.ivf` available via image copy or read-only volume mount.
  - [ ] API `/ready` remains non-2xx until the binary index is loaded.

  **QA Scenarios**:
  ```
  Scenario: Compose validates topology
    Tool: Bash
    Steps: docker compose config
    Expected: No config errors; services include haproxy, api01, api02.
    Evidence: .sisyphus/evidence/task-8-compose-config.txt

  Scenario: No forbidden modes
    Tool: Bash
    Steps: sh -c '! docker compose config | grep -E "network_mode: host|privileged: true"'
    Expected: No forbidden host/privileged configuration appears.
    Evidence: .sisyphus/evidence/task-8-compose-guardrails.txt
  ```

  **Commit**: YES | Message: `chore: add haproxy compose topology` | Files: `Dockerfile`, `docker-compose.yml`, `haproxy/**`

- [x] 9. Add benchmark binaries and measurement workflow

  **What to do**: Add benchmark coverage for vectorization, IVF search, HTTP handler, index loading, and memory/RSS reporting. Add `cmd/bench` or Makefile/script targets that produce evidence files. Update ADRs if measurement contradicts assumptions.
  **Must NOT do**: Do not optimize blindly or change algorithm decisions without updating ADRs.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` - Reason: benchmarking and measurement.
  - Skills: [`extreme-software-optimization`, `golang-pro`] - evidence-first performance work.
  - Omitted: [`tdd`] - benchmarks rather than red/green unit tests.

  **Parallelization**: Can Parallel: YES | Wave 4 | Blocks: T10 | Blocked By: T6, T7

  **References**:
  - Pattern: `.context/rinha-de-backend-2026/docs/en/EVALUATION.md:56-112` - p99 and detection scoring.
  - Pattern: `.context/rinha-de-backend-2026/docs/en/EVALUATION.md:186-188` - measure before complicating search.

  **Acceptance Criteria**:
  - [ ] `make bench` runs all benchmarks.
  - [ ] Benchmark output includes vectorization, IVF search, API handler, and index load.
  - [ ] Memory measurement evidence is saved under `.sisyphus/evidence/`.

  **QA Scenarios**:
  ```
  Scenario: Benchmark suite runs
    Tool: Bash
    Steps: make bench
    Expected: Benchmark output contains ns/op, B/op, allocs/op.
    Evidence: .sisyphus/evidence/task-9-bench.txt

  Scenario: Memory budget evidence exists
    Tool: Bash
    Steps: make measure-memory
    Expected: Output records API RSS and index size; API stays within configured limit during local run.
    Evidence: .sisyphus/evidence/task-9-memory.txt
  ```

  **Commit**: YES | Message: `chore: add benchmark workflow` | Files: `cmd/bench/**`, `Makefile`, `internal/**/**/*_test.go`, `docs/adr/*`

- [x] 10. Run local integration, smoke, and submission checks

  **What to do**: Add/update `info.json`, run full local stack, verify `/ready`, POST sample payloads, run official smoke test from `.context`, optionally run k6 load test if local resources allow. Update docs/ADRs for any final measured changes.
  **Must NOT do**: Do not claim completion without HTTP API evidence.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` - Reason: end-to-end verification.
  - Skills: [`verification-before-completion`, `devops-engineer`] - evidence before assertions.
  - Omitted: [`playwright`] - no browser UI.

  **Parallelization**: Can Parallel: NO | Wave 4 | Blocks: final verification | Blocked By: T8, T9

  **References**:
  - Pattern: `.context/rinha-de-backend-2026/docs/en/SUBMISSION.md:42-80` - submission files and `info.json`.
  - Test: `.context/rinha-de-backend-2026/test/smoke.js` - response-shape smoke test.

  **Acceptance Criteria**:
  - [ ] `docker compose up --build` starts all services.
  - [ ] `curl -fsS http://localhost:9999/ready` returns 2xx.
  - [ ] `POST /fraud-score` with example payload returns HTTP 200 and valid JSON.
  - [ ] Official smoke script passes against local stack.
  - [ ] `info.json` exists and contains stack metadata.

  **QA Scenarios**:
  ```
  Scenario: End-to-end HTTP stack works
    Tool: Bash
    Steps: sh -c 'docker compose up --build -d; status=0; curl -fsS http://localhost:9999/ready || status=$?; docker compose down; exit $status'
    Expected: Ready endpoint returns 2xx through HAProxy.
    Evidence: .sisyphus/evidence/task-10-ready.txt

  Scenario: Official smoke test passes
    Tool: Bash
    Steps: sh -c 'command -v k6 && docker compose up --build -d && k6 run .context/rinha-de-backend-2026/test/smoke.js; status=$?; docker compose down; exit $status'
    Expected: k6 checks pass with HTTP 200 and correct JSON shape.
    Evidence: .sisyphus/evidence/task-10-smoke.txt
  ```

  **Commit**: YES | Message: `chore: verify local submission stack` | Files: `info.json`, `docs/**`, `docker-compose.yml`, evidence references as needed

## Final Verification Wave (MANDATORY — after ALL implementation tasks)
> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.
> **Do NOT auto-proceed after verification. Wait for user's explicit approval before marking work complete.**
> **Never mark F1-F4 as checked before getting user's okay.** Rejection or user feedback -> fix -> re-run -> present again -> wait for okay.
- [x] F1. Plan Compliance Audit — oracle
- [x] F2. Code Quality Review — unspecified-high
- [x] F3. Real Manual QA — unspecified-high (+ HTTP/k6, no browser required)
- [x] F4. Scope Fidelity Check — deep

## Commit Strategy
- Commit after each task if verification passes and the user approves commits.
- Use conventional commit messages under 50 characters.
- Never use `git add .`; stage only intended files.

## Success Criteria
- ADRs document every major architectural decision.
- Implementation follows ADRs or updates them before changing direction.
- Local API returns valid decisions through HAProxy on port 9999.
- Unit tests cover required fraud-scoring edge cases.
- Benchmarks provide latency and memory evidence.
- Docker compose satisfies Rinha topology/resource/network constraints.
