package api

import (
	"context"
	"net/http"

	"github.com/example/distributed-go-network-controller/backend/internal/devices"
	"github.com/example/distributed-go-network-controller/backend/internal/jobs"
)

type DeploymentRepository interface {
	CreateDeployment(ctx context.Context, rawConfig string) (string, error)
	CreateJobsForDeployment(ctx context.Context, deploymentID string, devices []devices.Device) error
	GetDeployments(ctx context.Context) ([]jobs.Deployment, error)
	GetDeployment(ctx context.Context, id string) (*jobs.Deployment, error)
	GetJobs(ctx context.Context) ([]jobs.Job, error)
	GetJobsByDeployment(ctx context.Context, deploymentID string) ([]jobs.Job, error)
	ListAgents(ctx context.Context) ([]jobs.Agent, error)
	GetDeviceState(ctx context.Context, deviceName string) (*jobs.DeviceState, error)
	ListDeviceStates(ctx context.Context) ([]jobs.DeviceState, error)
	UpsertDeviceState(ctx context.Context, deviceName string, deviceType string, actualConfig []byte) error
}

func RegisterRoutes(mux *http.ServeMux, repository DeploymentRepository) {
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /validate", validateHandler)
	mux.HandleFunc("POST /deployments", createDeploymentHandler(repository))
	mux.HandleFunc("GET /deployments", listDeploymentsHandler(repository))
	mux.HandleFunc("GET /deployments/{id}", getDeploymentHandler(repository))
	mux.HandleFunc("GET /deployments/{id}/jobs", getDeploymentJobsHandler(repository))
	mux.HandleFunc("GET /jobs", listJobsHandler(repository))
	mux.HandleFunc("GET /agents", listAgentsHandler(repository))
	mux.HandleFunc("GET /devices", listDevicesHandler(repository))
	mux.HandleFunc("GET /devices/{name}", getDeviceHandler(repository))
	mux.HandleFunc("POST /devices/{name}/mutate", mutateDeviceHandler(repository))
}
