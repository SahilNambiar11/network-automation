package config

import "os"

type Config struct {
	DatabaseURL string
	ServicePort string
	WorkerID    string
}

func Load() Config {
	return Config{
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/network_controller?sslmode=disable"),
		ServicePort: getEnv("SERVICE_PORT", "8080"),
		WorkerID:    getEnv("WORKER_ID", defaultWorkerID()),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func defaultWorkerID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "worker-1"
	}

	return hostname
}
