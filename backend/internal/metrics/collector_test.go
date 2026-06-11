package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/distributed-go-network-controller/backend/internal/drift"
	"github.com/example/distributed-go-network-controller/backend/internal/jobs"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestMetricsEndpointReturnsPrometheusMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(NewCollector(fakeRepository{
		deployments: []jobs.Deployment{{ID: "deployment-1"}},
		jobs: []jobs.Job{
			{ID: "job-1", Status: jobs.JobStatusSuccess},
			{ID: "job-2", Status: jobs.JobStatusPending},
		},
		agents: []jobs.Agent{
			{ID: "worker-1", Status: jobs.AgentStatusHealthy, LastHeartbeat: time.Now(), ActiveJobs: 2},
		},
		driftReports: []drift.DriftReport{
			{DeviceName: "core-router", Drift: true},
			{DeviceName: "access-switch", Drift: false},
		},
	}))

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}

	body := response.Body.String()
	for _, metricName := range []string{
		"deployments_total",
		"jobs_total",
		"jobs_success_total",
		"jobs_pending_total",
		"active_agents",
		"devices_with_drift",
		"worker_active_jobs",
	} {
		if !strings.Contains(body, metricName) {
			t.Fatalf("expected metrics output to contain %q, got:\n%s", metricName, body)
		}
	}
}

type fakeRepository struct {
	deployments  []jobs.Deployment
	jobs         []jobs.Job
	agents       []jobs.Agent
	driftReports []drift.DriftReport
}

func (r fakeRepository) GetDeployments(ctx context.Context) ([]jobs.Deployment, error) {
	return r.deployments, nil
}

func (r fakeRepository) GetJobs(ctx context.Context) ([]jobs.Job, error) {
	return r.jobs, nil
}

func (r fakeRepository) ListAgents(ctx context.Context) ([]jobs.Agent, error) {
	return r.agents, nil
}

func (r fakeRepository) GenerateAllDriftReports(ctx context.Context) ([]drift.DriftReport, error) {
	return r.driftReports, nil
}
