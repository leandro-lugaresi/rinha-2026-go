// Command bench is a standalone benchmarking program that measures IVF index
// performance and memory usage outside the Go test framework. It reports:
//   - Index load time
//   - IVF search latency (p50, p99) over 1000 random queries
//   - Full fraud-score pipeline latency (vectorize + search + score)
//   - Go heap statistics (runtime.ReadMemStats)
//   - Process RSS (cross-platform)
package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/leandro-lugaresi/rinha-2026-go/internal/reference"
	"github.com/leandro-lugaresi/rinha-2026-go/internal/scoring"
	"github.com/leandro-lugaresi/rinha-2026-go/internal/search"
	"github.com/leandro-lugaresi/rinha-2026-go/internal/vectorize"
)

var (
	indexPath    = flag.String("index", "var/references.ivf", "Path to IVF index binary file")
	queryCount   = flag.Int("n", 1000, "Number of random queries for search benchmark")
	k            = flag.Int("k", 5, "Number of nearest neighbors to retrieve")
	silent       = flag.Bool("q", false, "Quiet mode: only print raw numbers (for automated measurement)")
)

func main() {
	flag.Parse()
	if _, err := os.Stat(*indexPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "FATAL: Index file not found: %s\n", *indexPath)
		fmt.Fprintf(os.Stderr, "Run: go run ./cmd/indexer -input <refs.json.gz> -output %s\n", *indexPath)
		os.Exit(1)
	}
	logf("=== Fraud Detection Benchmark ===\n\n")
	logf("Config: index=%s queries=%d k=%d\n", *indexPath, *queryCount, *k)

	// ─── 1. Index load ───────────────────────────────────────────
	logf("\n─── Index Load ───\n")
	loadStart := time.Now()
	idx, err := reference.LoadIndex(*indexPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
	defer idx.Close()
	loadDur := time.Since(loadStart)

	hdr := idx.Header()
	logf("  File:       %s\n", *indexPath)
	logf("  Vectors:    %d\n", hdr.NumVectors)
	logf("  Partitions: %d\n", hdr.Partitions)
	logf("  Dimensions: %d\n", hdr.Dimensions)
	logf("  Encoding:   uint%d\n", hdr.Encoding*8)
	logf("  Load time:  %v\n", loadDur)

	fi, err := os.Stat(*indexPath)
	if err == nil {
		logf("  File size:  %.1f MB\n", float64(fi.Size())/(1024*1024))
	}

	// ─── 2. IVF Search benchmark ─────────────────────────────────
	logf("\n─── IVF Search (n=%d, k=%d) ───\n", *queryCount, *k)
	searcher := search.NewIVFSearcher(idx, 0)
	rng := rand.New(rand.NewPCG(42, 99))
	queries := genQueries(*queryCount, rng)

	latencies := make([]float64, *queryCount)
	for i := 0; i < *queryCount; i++ {
		start := time.Now()
		results, err := searcher.Search(queries[i], *k)
		latencies[i] = float64(time.Since(start).Microseconds())
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: search %d: %v\n", i, err)
			os.Exit(1)
		}
		if len(results) != *k {
			fmt.Fprintf(os.Stderr, "FATAL: search %d: expected %d results, got %d\n", i, *k, len(results))
			os.Exit(1)
		}
	}
	reportLatency("IVF Search", latencies, *queryCount)

	// ─── 3. Full fraud-score pipeline ────────────────────────────
	logf("\n─── Full Fraud-Score Pipeline (vectorize + search + score, n=%d) ───\n", *queryCount)
	payloads := genPayloads(*queryCount, rng)
	pipeLatencies := make([]float64, *queryCount)
	for i := 0; i < *queryCount; i++ {
		start := time.Now()
		vec, err := vectorize.Vectorize(payloads[i])
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: vectorize %d: %v\n", i, err)
			os.Exit(1)
		}
		results, err := searcher.Search(vec, *k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: search %d: %v\n", i, err)
			os.Exit(1)
		}
		labels := make([]string, len(results))
		for j, ref := range results {
			labels[j] = ref.Label
		}
		scoring.ComputeScore(labels)
		pipeLatencies[i] = float64(time.Since(start).Microseconds())
	}
	reportLatency("Full Pipeline", pipeLatencies, *queryCount)

	// ─── 4. Memory report ────────────────────────────────────────
	logf("\n─── Memory Report ───\n")
	printMemoryStats()
}

// ─── Helpers ────────────────────────────────────────────────────────

func logf(format string, args ...interface{}) {
	if !*silent {
		fmt.Printf(format, args...)
	}
}

// randomQuery generates a 14-dimensional vector with values in [-1, 1].
// Dimensions 0-4 and 7-13 are clamped to [0, 1]; dimensions 5-6 may be -1.
func randomQuery(rng *rand.Rand) [14]float64 {
	var q [14]float64
	for i := 0; i < 14; i++ {
		switch {
		case i == 5 || i == 6:
			q[i] = rng.Float64()*2 - 1
		default:
			q[i] = rng.Float64()
		}
	}
	return q
}

func genQueries(n int, rng *rand.Rand) [][14]float64 {
	qs := make([][14]float64, n)
	for i := range qs {
		qs[i] = randomQuery(rng)
	}
	return qs
}

func genPayloads(n int, rng *rand.Rand) []vectorize.Payload {
	mccs := []string{"5411", "5812", "5912", "5944", "7801", "5999"}
	payloads := make([]vectorize.Payload, n)
	for i := range payloads {
		payloads[i] = vectorize.Payload{
			Transaction: vectorize.Transaction{
				Amount:       rng.Float64() * 10000,
				Installments: rng.IntN(24) + 1,
				RequestedAt:  "2026-01-01T12:00:00Z",
			},
			Customer: vectorize.Customer{
				AvgAmount:  rng.Float64() * 5000,
				TxCount24h: rng.IntN(50),
			},
			Merchant: vectorize.Merchant{
				MCC:       mccs[rng.IntN(len(mccs))],
				AvgAmount: rng.Float64() * 8000,
			},
			Terminal: vectorize.Terminal{
				IsOnline:    rng.IntN(2) == 0,
				CardPresent: rng.IntN(2) == 0,
				KmFromHome:  rng.Float64() * 500,
			},
			LastTransaction: &vectorize.LastTransaction{
				Timestamp:     "2026-01-01T11:00:00Z",
				KmFromCurrent: rng.Float64() * 200,
			},
		}
	}
	return payloads
}

func reportLatency(name string, latencies []float64, n int) {
	sort.Float64s(latencies)

	var sum float64
	for _, l := range latencies {
		sum += l
	}
	avg := sum / float64(n)

	p50Idx := int(math.Floor(float64(n) * 0.50))
	if p50Idx >= n {
		p50Idx = n - 1
	}
	p90Idx := int(math.Floor(float64(n) * 0.90))
	if p90Idx >= n {
		p90Idx = n - 1
	}
	p99Idx := int(math.Floor(float64(n) * 0.99))
	if p99Idx >= n {
		p99Idx = n - 1
	}

	logf("  %-16s avg=%.1fµs  p50=%.1fµs  p90=%.1fµs  p99=%.1fµs  min=%.1fµs  max=%.1fµs\n",
		name, avg, latencies[p50Idx], latencies[p90Idx], latencies[p99Idx], latencies[0], latencies[n-1])

	// Also print raw numbers for machine parsing (even in non-silent mode)
	fmt.Printf("  RAW|%s|avg_us=%.1f|p50_us=%.1f|p90_us=%.1f|p99_us=%.1f|min_us=%.1f|max_us=%.1f\n",
		name, avg, latencies[p50Idx], latencies[p90Idx], latencies[p99Idx], latencies[0], latencies[n-1])
}

func printMemoryStats() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	logf("  Go heap alloc:    %.2f MB\n", float64(ms.HeapAlloc)/(1024*1024))
	logf("  Go heap sys:      %.2f MB\n", float64(ms.HeapSys)/(1024*1024))
	logf("  Go heap inuse:    %.2f MB\n", float64(ms.HeapInuse)/(1024*1024))
	logf("  Go heap idle:     %.2f MB\n", float64(ms.HeapIdle)/(1024*1024))
	logf("  Go total alloc:   %.2f MB\n", float64(ms.TotalAlloc)/(1024*1024))
	logf("  Go stack sys:     %.2f MB\n", float64(ms.StackSys)/(1024*1024))
	logf("  Go stack inuse:   %.2f MB\n", float64(ms.StackInuse)/(1024*1024))
	logf("  Go num GC cycles: %d\n", ms.NumGC)
	logf("  Go pause total:   %v\n", time.Duration(ms.PauseTotalNs))
	logf("  Go goroutines:    %d\n", runtime.NumGoroutine())

	rssKB := measureRSS()
	if rssKB > 0 {
		logf("  Process RSS:      %.2f MB\n", float64(rssKB)/1024)
	}
	fmt.Printf("  RAW|memory|heap_alloc_mb=%.2f|heap_sys_mb=%.2f|heap_inuse_mb=%.2f|rss_mb=%.2f|goroutines=%d\n",
		float64(ms.HeapAlloc)/(1024*1024),
		float64(ms.HeapSys)/(1024*1024),
		float64(ms.HeapInuse)/(1024*1024),
		float64(rssKB)/1024,
		runtime.NumGoroutine())
}

// measureRSS returns the process RSS in kilobytes, or 0 if unavailable.
// On Linux, it reads /proc/self/status. On macOS/BSDs, it uses ps.
func measureRSS() int64 {
	pid := os.Getpid()

	// Linux: read /proc/self/status
	if data, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					kb, err := strconv.ParseInt(fields[1], 10, 64)
					if err == nil {
						return kb
					}
				}
			}
		}
	}

	// macOS/BSDs: use ps
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err == nil {
		kb, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if err == nil {
			return kb
		}
	}

	return 0
}
