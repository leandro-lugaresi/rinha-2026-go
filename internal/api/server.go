// Package api provides the HTTP server for the fraud detection API.
package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/leandro-lugaresi/rinha-2026-go/internal/reference"
	"github.com/leandro-lugaresi/rinha-2026-go/internal/scoring"
	"github.com/leandro-lugaresi/rinha-2026-go/internal/search"
	"github.com/leandro-lugaresi/rinha-2026-go/internal/vectorize"
)

// Server holds the IVF searcher and readiness state for the fraud-score API.
type Server struct {
	searcher *search.IVFSearcher
	index    *reference.IVFIndex
	ready    bool
}

// FraudScoreResponse is the JSON response for the /fraud-score endpoint.
type FraudScoreResponse struct {
	Approved   bool    `json:"approved"`
	FraudScore float64 `json:"fraud_score"`
}

// NewServer loads the IVF index from indexPath and creates an IVFSearcher.
// The server is NOT marked ready until MarkReady is called.
func NewServer(indexPath string) (*Server, error) {
	idx, err := reference.LoadIndex(indexPath)
	if err != nil {
		return nil, fmt.Errorf("load index: %w", err)
	}
	searcher := search.NewIVFSearcher(idx, 0)
	return &Server{
		searcher: searcher,
		index:    idx,
		ready:    false,
	}, nil
}

// MarkReady sets the server's readiness state to true.
func (s *Server) MarkReady() {
	s.ready = true
}

// Close releases resources held by the index.
func (s *Server) Close() error {
	if s.index == nil {
		return nil
	}
	return s.index.Close()
}

// ReadyHandler responds to GET /ready. Returns 200 only when the server
// has been marked ready (index loaded, searcher primed).
func (s *Server) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	if !s.ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ready"}`)
}

// FraudScoreHandler handles POST /fraud-score. It decodes the request payload,
// vectorizes it, searches for the 5 nearest neighbors, computes the fraud
// score, and returns the decision.
//
// On any error (decode, vectorize, search, or encode), it logs the error
// and returns HTTP 200 with the safe default {"approved":true,"fraud_score":0.0}.
func (s *Server) FraudScoreHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var payload vectorize.Payload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("error decoding payload: %v", err)
		writeDefaultResponse(w)
		return
	}

	vec, err := vectorize.Vectorize(payload)
	if err != nil {
		log.Printf("error vectorizing payload: %v", err)
		writeDefaultResponse(w)
		return
	}

	results, err := s.searcher.Search(vec, 5)
	if err != nil {
		log.Printf("error searching: %v", err)
		writeDefaultResponse(w)
		return
	}

	labels := make([]string, len(results))
	for i, ref := range results {
		labels[i] = ref.Label
	}

	fraudScore, approved := scoring.ComputeScore(labels)

	resp := FraudScoreResponse{
		Approved:   approved,
		FraudScore: fraudScore,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("error encoding response: %v", err)
		writeDefaultResponse(w)
		return
	}
}

// writeDefaultResponse writes the safe-default fraud-score response
// (approved=true, fraud_score=0.0) to w as JSON.
func writeDefaultResponse(w http.ResponseWriter) {
	if err := json.NewEncoder(w).Encode(FraudScoreResponse{
		Approved:   true,
		FraudScore: 0.0,
	}); err != nil {
		log.Printf("error encoding default response: %v", err)
	}
}
