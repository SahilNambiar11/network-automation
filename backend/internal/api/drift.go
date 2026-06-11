package api

import (
	"net/http"

	"github.com/example/distributed-go-network-controller/backend/internal/drift"
)

func listDriftHandler(repository DeploymentRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reports, err := repository.GenerateAllDriftReports(r.Context())
		if err != nil {
			http.Error(w, "failed to generate drift reports", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, reports)
	}
}

func getDeviceDriftHandler(repository DeploymentRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report, err := repository.GenerateDeviceDrift(r.Context(), r.PathValue("device"))
		if err != nil {
			http.Error(w, "failed to generate drift report", http.StatusInternalServerError)
			return
		}
		if report == nil {
			http.NotFound(w, r)
			return
		}

		writeJSON(w, http.StatusOK, report)
	}
}

func driftSummaryHandler(repository DeploymentRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reports, err := repository.GenerateAllDriftReports(r.Context())
		if err != nil {
			http.Error(w, "failed to generate drift summary", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, drift.SummarizeReports(reports))
	}
}
