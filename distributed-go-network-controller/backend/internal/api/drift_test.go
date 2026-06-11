package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/distributed-go-network-controller/backend/internal/drift"
)

func TestDriftSummaryHandlerReturnsCounts(t *testing.T) {
	repository := &fakeDeploymentRepository{
		driftReports: []drift.DriftReport{
			{DeviceName: "core-router", Drift: true},
			{DeviceName: "access-switch", Drift: false},
		},
	}
	request := httptest.NewRequest(http.MethodGet, "/drift/summary", nil)
	response := httptest.NewRecorder()

	driftSummaryHandler(repository).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}

	var payload drift.DriftSummary
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.DevicesChecked != 2 {
		t.Fatalf("expected 2 devices checked, got %d", payload.DevicesChecked)
	}
	if payload.DevicesWithDrift != 1 {
		t.Fatalf("expected 1 device with drift, got %d", payload.DevicesWithDrift)
	}
}
