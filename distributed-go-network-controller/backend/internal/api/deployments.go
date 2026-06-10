package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/example/distributed-go-network-controller/backend/internal/devices"
	"github.com/example/distributed-go-network-controller/backend/internal/jobs"
	"github.com/example/distributed-go-network-controller/backend/internal/validation"
)

type createDeploymentResponse struct {
	Valid        bool                         `json:"valid"`
	DeploymentID string                       `json:"deployment_id,omitempty"`
	JobsCreated  int                          `json:"jobs_created,omitempty"`
	Errors       []validation.ValidationError `json:"errors,omitempty"`
}

func createDeploymentHandler(repository DeploymentRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		validationResponse := validation.ValidateNetworkConfig(config)
		if !validationResponse.Valid {
			writeJSON(w, http.StatusOK, createDeploymentResponse{
				Valid:  false,
				Errors: validationResponse.Errors,
			})
			return
		}

		deploymentID, err := repository.CreateDeployment(r.Context(), string(body))
		if err != nil {
			http.Error(w, "failed to create deployment", http.StatusInternalServerError)
			return
		}

		if err := repository.CreateJobsForDeployment(r.Context(), deploymentID, config.Devices); err != nil {
			http.Error(w, "failed to create deployment jobs", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, createDeploymentResponse{
			Valid:        true,
			DeploymentID: deploymentID,
			JobsCreated:  len(config.Devices),
		})
	}
}

func listDeploymentsHandler(repository DeploymentRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deployments, err := repository.GetDeployments(r.Context())
		if err != nil {
			http.Error(w, "failed to get deployments", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, deployments)
	}
}

func getDeploymentHandler(repository DeploymentRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deployment, err := repository.GetDeployment(r.Context(), r.PathValue("id"))
		if err != nil {
			http.Error(w, "failed to get deployment", http.StatusInternalServerError)
			return
		}
		if deployment == nil {
			http.NotFound(w, r)
			return
		}

		writeJSON(w, http.StatusOK, deployment)
	}
}

func getDeploymentJobsHandler(repository DeploymentRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobs, err := repository.GetJobsByDeployment(r.Context(), r.PathValue("id"))
		if err != nil {
			http.Error(w, "failed to get deployment jobs", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, jobs)
	}
}

func listJobsHandler(repository DeploymentRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobs, err := repository.GetJobs(r.Context())
		if err != nil {
			http.Error(w, "failed to get jobs", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, jobs)
	}
}

func listAgentsHandler(repository DeploymentRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agents, err := repository.ListAgents(r.Context())
		if err != nil {
			http.Error(w, "failed to get agents", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, jobs.AgentsWithComputedHealth(agents, time.Now()))
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}
