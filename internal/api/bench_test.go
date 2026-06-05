package api_test

import (
	"encoding/binary"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/leandro-lugaresi/rinha-2026-go/internal/api"
	"github.com/leandro-lugaresi/rinha-2026-go/internal/reference"
)

func simpleKMeansBench(tb testing.TB, refs []reference.Reference, k int) ([][14]float32, []int) {
	tb.Helper()

	centroids := make([][14]float32, k)
	assignments := make([]int, len(refs))

	if len(refs) == 0 {
		return centroids, assignments
	}

	step := float64(len(refs)) / float64(k)
	for p := 0; p < k; p++ {
		idx := int(float64(p) * step)
		if idx >= len(refs) {
			idx = len(refs) - 1
		}
		for d := 0; d < 14; d++ {
			centroids[p][d] = float32(refs[idx].Vector[d])
		}
	}

	for iter := 0; iter < 10; iter++ {
		for i, r := range refs {
			bestDist := math.MaxFloat64
			bestP := 0
			for p := 0; p < k; p++ {
				var d float64
				for j := 0; j < 14; j++ {
					diff := r.Vector[j] - float64(centroids[p][j])
					d += diff * diff
				}
				if d < bestDist {
					bestDist = d
					bestP = p
				}
			}
			assignments[i] = bestP
		}

		sums := make([][14]float64, k)
		counts := make([]int, k)
		for i, r := range refs {
			p := assignments[i]
			for d := 0; d < 14; d++ {
				sums[p][d] += r.Vector[d]
			}
			counts[p]++
		}
		for p := 0; p < k; p++ {
			if counts[p] > 0 {
				n := float64(counts[p])
				for d := 0; d < 14; d++ {
					centroids[p][d] = float32(sums[p][d] / n)
				}
			}
		}
	}

	return centroids, assignments
}

func writeMiniIVFBench(tb testing.TB, refs []reference.Reference, centroids [][14]float32, assignments []int) string {
	tb.Helper()

	if len(refs) != len(assignments) {
		tb.Fatalf("refs (%d) and assignments (%d) must match", len(refs), len(assignments))
	}
	numParts := len(centroids)
	if numParts == 0 {
		numParts = 1
	}

	counts := make([]uint32, numParts)
	for _, a := range assignments {
		if a < 0 || a >= numParts {
			tb.Fatalf("partition assignment %d out of range [0, %d)", a, numParts)
		}
		counts[a]++
	}

	offsets := make([]uint32, numParts)
	var o uint32
	for p := 0; p < numParts; p++ {
		offsets[p] = o
		o += counts[p] * 14
	}

	vecsByPart := make([][]reference.Reference, numParts)
	for i, r := range refs {
		p := assignments[i]
		vecsByPart[p] = append(vecsByPart[p], r)
	}

	f, err := os.CreateTemp("", "bench-ivf-*.bin")
	if err != nil {
		tb.Fatalf("create temp file: %v", err)
	}
	path := f.Name()

	writeLE := func(v uint32) {
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], v)
		f.Write(buf[:])
	}

	f.Write([]byte("RVI1"))
	writeLE(uint32(1))
	writeLE(uint32(numParts))
	writeLE(uint32(len(refs)))
	writeLE(uint32(14))
	writeLE(uint32(1))

	for p := 0; p < numParts; p++ {
		for d := 0; d < 14; d++ {
			writeLE(math.Float32bits(centroids[p][d]))
		}
		writeLE(offsets[p])
		writeLE(counts[p])
	}

	for p := 0; p < numParts; p++ {
		for _, r := range vecsByPart[p] {
			q := reference.QuantizeVector(r.Vector)
			f.Write(q[:])
		}
	}

	for p := 0; p < numParts; p++ {
		for _, r := range vecsByPart[p] {
			var l uint8
			if r.Label == "fraud" {
				l = 1
			}
			f.Write([]byte{l})
		}
	}

	f.Close()
	return path
}

func newBenchServer(tb testing.TB) *api.Server {
	tb.Helper()

	refs := loadFixture(tb)
	if len(refs) < 20 {
		tb.Skip("need at least 20 references")
	}

	centroids, assignments := simpleKMeansBench(tb, refs, 4)
	path := writeMiniIVFBench(tb, refs, centroids, assignments)
	tb.Cleanup(func() { os.Remove(path) })

	srv, err := api.NewServer(path)
	if err != nil {
		tb.Fatalf("NewServer: %v", err)
	}
	tb.Cleanup(func() { srv.Close() })
	return srv
}

var benchPayload = `{
	"id": "tx-bench-001",
	"transaction": {
		"amount": 100.0,
		"installments": 3,
		"requested_at": "2026-03-11T20:23:35Z"
	},
	"customer": {
		"avg_amount": 200.0,
		"tx_count_24h": 3,
		"known_merchants": ["MERC-001"]
	},
	"merchant": {
		"id": "MERC-001",
		"mcc": "5912",
		"avg_amount": 150.0
	},
	"terminal": {
		"is_online": false,
		"card_present": true,
		"km_from_home": 13.7
	},
	"last_transaction": {
		"timestamp": "2026-03-11T14:58:35Z",
		"km_from_current": 18.8
	}
}`

func BenchmarkFraudScoreHandler(b *testing.B) {
	srv := newBenchServer(b)
	srv.MarkReady()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/fraud-score", strings.NewReader(benchPayload))
		rec := httptest.NewRecorder()
		srv.FraudScoreHandler(rec, req)

		if rec.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}
}
