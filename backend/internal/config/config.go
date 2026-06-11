package config

import (
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL       string
	ServicePort       string
	WorkerID          string
	WorkerConcurrency int
}

func Load() Config {
	return Config{
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/network_controller?sslmode=disable"),
		ServicePort:       getEnv("SERVICE_PORT", "8080"),
		WorkerID:          getEnv("WORKER_ID", defaultWorkerID()),
		WorkerConcurrency: getEnvInt("WORKER_CONCURRENCY", 3),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}

	return parsed
}

func defaultWorkerID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "worker-1"
	}

	return hostname
}
