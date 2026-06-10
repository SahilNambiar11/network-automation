package main

import (
	"log"
	"net/http"

	"github.com/example/distributed-go-network-controller/backend/internal/api"
	"github.com/example/distributed-go-network-controller/backend/internal/config"
)

func main() {
	cfg := config.Load()

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	addr := ":" + cfg.ServicePort
	log.Printf("controller listening on %s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("controller stopped: %v", err)
	}
}
