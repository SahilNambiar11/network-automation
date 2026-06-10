package main

import (
	"log"
	"time"

	"github.com/example/distributed-go-network-controller/backend/internal/config"
)

func main() {
	cfg := config.Load()

	log.Printf("worker starting with id %q", cfg.WorkerID)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		log.Println("worker heartbeat")
		<-ticker.C
	}
}
