.PHONY: test bench build run docker-build smoke clean fmt vet measure-memory profile-cpu profile-mem

# ─── Metadata ────────────────────────────────────────────────
APP      := rinha-2026-go
GO       := go
GOFLAGS  := -ldflags="-s -w"
BINDIR   := bin
EVIDENCE := .sisyphus/evidence
NOTEPAD  := .sisyphus/notepads/rinha-2026-go-backend

# ─── Testing ──────────────────────────────────────────────────
test: fmt vet
	$(GO) test ./... -count=1 -race

# Standard Go test benchmarks (table-driven, -benchmem)
bench:
	$(GO) test -bench=. -benchmem ./... -count=1

# Standalone benchmark binary (index load, IVF search, memory)
measure-memory: $(BINDIR)/bench
	mkdir -p $(EVIDENCE)
	./$(BINDIR)/bench -index var/references.ivf -n 1000 | tee $(EVIDENCE)/task-9-memory.txt

$(BINDIR)/bench:
	$(GO) build $(GOFLAGS) -o $(BINDIR)/bench ./cmd/bench

# Profile: CPU flamegraph input
profile-cpu:
	$(GO) test -bench=. -benchmem ./... -cpuprofile=$(EVIDENCE)/cpu.prof -count=1
	@echo "CPU profile written to $(EVIDENCE)/cpu.prof"
	@echo "View: go tool pprof -http=:8080 $(EVIDENCE)/cpu.prof"

# Profile: memory/heap allocation
profile-mem:
	$(GO) test -bench=. -benchmem ./... -memprofile=$(EVIDENCE)/mem.prof -count=1
	@echo "Memory profile written to $(EVIDENCE)/mem.prof"
	@echo "View: go tool pprof -http=:8080 $(EVIDENCE)/mem.prof"

# ─── Static analysis ──────────────────────────────────────────
fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

# ─── Build ────────────────────────────────────────────────────
build: $(BINDIR)/api $(BINDIR)/indexer $(BINDIR)/bench

$(BINDIR)/api:
	$(GO) build $(GOFLAGS) -o $(BINDIR)/api ./cmd/api

$(BINDIR)/indexer:
	$(GO) build $(GOFLAGS) -o $(BINDIR)/indexer ./cmd/indexer

# ─── Run ──────────────────────────────────────────────────────
run: $(BINDIR)/api
	./$(BINDIR)/api

# ─── Container ────────────────────────────────────────────────
docker-build:
	docker build -t rinha-2026-go:latest .

# ─── Smoke test ───────────────────────────────────────────────
smoke:
	@echo "==> Checking /ready..."
	@curl -sf http://localhost:9999/ready > /dev/null && echo "  /ready: OK" || echo "  /ready: FAIL"
	@echo "==> Checking /fraud-score..."
	@curl -sf -X POST http://localhost:9999/fraud-score \
		-H 'Content-Type: application/json' \
		-d '{"id":"smoke-001","transaction":{"amount":100,"installments":3,"requested_at":"2026-03-11T20:23:35Z"},"customer":{"avg_amount":200,"tx_count_24h":3,"known_merchants":["MERC-001"]},"merchant":{"id":"MERC-001","mcc":"5912","avg_amount":150},"terminal":{"is_online":false,"card_present":true,"km_from_home":13.7},"last_transaction":{"timestamp":"2026-03-11T14:58:35Z","km_from_current":18.8}}' \
		> /dev/null && echo "  /fraud-score: OK" || echo "  /fraud-score: FAIL"

# ─── Clean ────────────────────────────────────────────────────
clean:
	rm -rf $(BINDIR)/
