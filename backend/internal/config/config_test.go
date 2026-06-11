package config

import "testing"

func TestLoadDefaultWorkerConcurrency(t *testing.T) {
	t.Setenv("WORKER_CONCURRENCY", "")

	cfg := Load()

	if cfg.WorkerConcurrency != 3 {
		t.Fatalf("expected default worker concurrency 3, got %d", cfg.WorkerConcurrency)
	}
}

func TestLoadCustomWorkerConcurrency(t *testing.T) {
	t.Setenv("WORKER_CONCURRENCY", "5")

	cfg := Load()

	if cfg.WorkerConcurrency != 5 {
		t.Fatalf("expected worker concurrency 5, got %d", cfg.WorkerConcurrency)
	}
}
