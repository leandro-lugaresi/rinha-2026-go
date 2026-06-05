package main

import (
	"log"
	"net/http"
	"os"

	"github.com/leandro-lugaresi/rinha-2026-go/internal/api"
)

func main() {
	indexPath := os.Getenv("RINHA_INDEX_PATH")
	if indexPath == "" {
		indexPath = "var/references.ivf"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9999"
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
	log.Printf("api listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
