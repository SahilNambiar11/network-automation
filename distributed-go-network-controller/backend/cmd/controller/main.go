package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/example/distributed-go-network-controller/backend/internal/api"
	"github.com/example/distributed-go-network-controller/backend/internal/config"
	"github.com/example/distributed-go-network-controller/backend/internal/db"
	"github.com/example/distributed-go-network-controller/backend/internal/jobs"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	database, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer database.Close()

	if err := db.RunMigrations(ctx, database, "migrations"); err != nil {
		log.Fatalf("failed to run database migrations: %v", err)
	}
	log.Println("database migrations are up to date")

	repository := jobs.NewRepository(database)

	mux := http.NewServeMux()
	api.RegisterRoutes(mux, repository)

	addr := ":" + cfg.ServicePort
	log.Printf("controller listening on %s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("controller stopped: %v", err)
	}
}
