package db

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	_ "github.com/lib/pq"
)

func Connect(ctx context.Context, databaseURL string) (*sql.DB, error) {
	if databaseURL == "" {
		return nil, errors.New("database URL is required")
	}

	const attempts = 30

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		conn, err := sql.Open("postgres", databaseURL)
		if err != nil {
			log.Printf("failed to open database connection: %v", err)
			return nil, err
		}

		if err := conn.PingContext(ctx); err == nil {
			log.Println("connected to postgres")
			return conn, nil
		} else {
			lastErr = err
			log.Printf("postgres connection attempt %d/%d failed: %v", attempt, attempts, err)
		}

		if err := conn.Close(); err != nil {
			log.Printf("failed to close unsuccessful database connection: %v", err)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	return nil, lastErr
}
