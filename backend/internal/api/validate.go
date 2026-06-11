package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/example/distributed-go-network-controller/backend/internal/devices"
	"github.com/example/distributed-go-network-controller/backend/internal/validation"
)

func validateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	config, err := devices.ParseYAML(body)
	if err != nil {
		http.Error(w, "invalid YAML", http.StatusBadRequest)
		return
	}

	response := validation.ValidateNetworkConfig(config)
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}
