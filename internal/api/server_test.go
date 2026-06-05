package api_test

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/leandro-lugaresi/rinha-2026-go/internal/api"
	"github.com/leandro-lugaresi/rinha-2026-go/internal/reference"
	"github.com/leandro-lugaresi/rinha-2026-go/internal/vectorize"
)

const fixturePath = "../../.context/rinha-de-backend-2026/resources/example-references.json"
const normalizationPath = "../../.context/rinha-de-backend-2026/resources/normalization.json"
const mccRiskPath = "../../.context/rinha-de-backend-2026/resources/mcc_risk.json"

const referenceLen = 14

func loadFixture(tb testing.TB) []reference.Reference {
	tb.Helper()
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		tb.Skipf("fixture file not found: %s", fixturePath)
	}
	refs, err := reference.LoadExampleReferences(fixturePath)
	if err != nil {
		tb.Fatalf("LoadExampleReferences: %v", err)
	}
	return refs
}

func simpleKMeans(t *testing.T, refs []reference.Reference, k int) ([][14]float32, []int) {
	t.Helper()

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
		for d := 0; d < referenceLen; d++ {
			centroids[p][d] = float32(refs[idx].Vector[d])
		}
	}

	for iter := 0; iter < 10; iter++ {
		for i, r := range refs {
			bestDist := math.MaxFloat64
			bestP := 0
			for p := 0; p < k; p++ {
				var d float64
				for j := 0; j < referenceLen; j++ {
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
			for d := 0; d < referenceLen; d++ {
				sums[p][d] += r.Vector[d]
			}
			counts[p]++
		}
		for p := 0; p < k; p++ {
			if counts[p] > 0 {
				n := float64(counts[p])
				for d := 0; d < referenceLen; d++ {
					centroids[p][d] = float32(sums[p][d] / n)
				}
			}
		}
	}

	return centroids, assignments
}

func writeMiniIVF(t *testing.T, refs []reference.Reference, centroids [][14]float32, assignments []int) string {
	t.Helper()

	if len(refs) != len(assignments) {
		t.Fatalf("refs (%d) and assignments (%d) must match", len(refs), len(assignments))
	}
	numParts := len(centroids)
	if numParts == 0 {
		numParts = 1
	}

	counts := make([]uint32, numParts)
	for _, a := range assignments {
		if a < 0 || a >= numParts {
			t.Fatalf("partition assignment %d out of range [0, %d)", a, numParts)
		}
		counts[a]++
	}

	offsets := make([]uint32, numParts)
	var o uint32
	for p := 0; p < numParts; p++ {
		offsets[p] = o
		o += counts[p] * referenceLen
	}

	vecsByPart := make([][]reference.Reference, numParts)
	for i, r := range refs {
		p := assignments[i]
		vecsByPart[p] = append(vecsByPart[p], r)
	}

	f, err := os.CreateTemp("", "test-ivf-*.bin")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
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
	writeLE(uint32(referenceLen))
	writeLE(uint32(1))

	for p := 0; p < numParts; p++ {
		for d := 0; d < referenceLen; d++ {
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

func newTestServer(t *testing.T) *api.Server {
	t.Helper()
	if err := vectorize.LoadResources(normalizationPath, mccRiskPath); err != nil {
		t.Fatalf("LoadResources: %v", err)
	}

	refs := loadFixture(t)
	if len(refs) < 20 {
		t.Skip("need at least 20 references")
	}

	centroids, assignments := simpleKMeans(t, refs, 4)
	path := writeMiniIVF(t, refs, centroids, assignments)
	t.Cleanup(func() { os.Remove(path) })

	srv, err := api.NewServer(path)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv
}

func TestAPI_ReadyBeforeLoad(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	srv.ReadyHandler(rec, req)

	if rec.Code >= 200 && rec.Code < 300 {
		t.Errorf("expected non-2xx before ready, got %d", rec.Code)
	}
}

func TestAPI_ReadyAfterLoad(t *testing.T) {
	srv := newTestServer(t)
	srv.MarkReady()

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	srv.ReadyHandler(rec, req)

	if rec.Code < 200 || rec.Code >= 300 {
		t.Errorf("expected 2xx after ready, got %d", rec.Code)
	}
}

func TestAPI_FraudScoreValidPayload(t *testing.T) {
	srv := newTestServer(t)
	srv.MarkReady()

	payload := `{
		"id": "tx-test-001",
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

	req := httptest.NewRequest(http.MethodPost, "/fraud-score", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	srv.FraudScoreHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp api.FraudScoreResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	if resp.FraudScore < 0.0 || resp.FraudScore > 1.0 {
		t.Errorf("fraud_score out of range [0,1]: %f", resp.FraudScore)
	}
	if !resp.Approved && resp.FraudScore < 0.6 {
		t.Errorf("approved=false but fraud_score=%.2f < 0.6", resp.FraudScore)
	}
	if resp.Approved && resp.FraudScore >= 0.6 {
		t.Errorf("approved=true but fraud_score=%.2f >= 0.6", resp.FraudScore)
	}
}

func TestAPI_FraudScoreMalformedPayload(t *testing.T) {
	srv := newTestServer(t)
	srv.MarkReady()

	req := httptest.NewRequest(http.MethodPost, "/fraud-score", strings.NewReader("not-json"))
	rec := httptest.NewRecorder()
	srv.FraudScoreHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 on malformed payload, got %d", rec.Code)
	}

	var resp api.FraudScoreResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	if !resp.Approved {
		t.Error("expected approved=true (safe default) on malformed payload")
	}
	if resp.FraudScore != 0.0 {
		t.Errorf("expected fraud_score=0.0 (safe default) on malformed payload, got %f", resp.FraudScore)
	}
}
