package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/leandro-lugaresi/rinha-2026-go/internal/api"
	"github.com/leandro-lugaresi/rinha-2026-go/internal/vectorize"
)

func main() {
	indexPath := os.Getenv("RINHA_INDEX_PATH")
	if indexPath == "" {
		indexPath = "var/references.ivf"
	}
	normalizationPath := os.Getenv("RINHA_NORMALIZATION_PATH")
	if normalizationPath == "" {
		normalizationPath = "var/normalization.json"
	}
	mccRiskPath := os.Getenv("RINHA_MCC_RISK_PATH")
	if mccRiskPath == "" {
		mccRiskPath = "var/mcc_risk.json"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9999"
	}

	if err := vectorize.LoadResources(normalizationPath, mccRiskPath); err != nil {
		log.Fatalf("failed to load vectorization resources: %v", err)
	}

	srv, err := api.NewServer(indexPath)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}
	srv.MarkReady()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ready", srv.ReadyHandler)
	mux.HandleFunc("POST /fraud-score", srv.FraudScoreHandler)

	addr := ":" + port
	log.Print("api listening")
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
