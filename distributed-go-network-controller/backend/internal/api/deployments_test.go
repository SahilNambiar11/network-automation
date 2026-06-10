package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/distributed-go-network-controller/backend/internal/devices"
	"github.com/example/distributed-go-network-controller/backend/internal/jobs"
)

func TestCreateDeploymentRejectsInvalidYAMLConfig(t *testing.T) {
	repository := &fakeDeploymentRepository{}
	request := httptest.NewRequest(http.MethodPost, "/deployments", bytes.NewBufferString(`
devices:
  - name: core-router
    type: router
vlans:
  - id: 10
    name: engineering
    subnet: invalid-cidr
`))
	response := httptest.NewRecorder()

	createDeploymentHandler(repository).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if repository.createDeploymentCalled {
		t.Fatalf("expected repository not to be called for invalid config")
	}

	var payload createDeploymentResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Valid {
		t.Fatalf("expected invalid response")
	}
	if len(payload.Errors) == 0 {
		t.Fatalf("expected validation errors")
	}
}

func TestCreateDeploymentCreatesJobsForValidConfig(t *testing.T) {
	repository := &fakeDeploymentRepository{deploymentID: "deployment-1"}
	request := httptest.NewRequest(http.MethodPost, "/deployments", bytes.NewBufferString(`
devices:
  - name: core-router
    type: router
  - name: access-switch
    type: switch
vlans:
  - id: 10
    name: engineering
    subnet: 10.10.0.0/24
firewall_rules:
  - source: internet
    destination: engineering
    port: 443
    action: allow
`))
	response := httptest.NewRecorder()

	createDeploymentHandler(repository).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}

	var payload createDeploymentResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Valid {
		t.Fatalf("expected valid response, got errors: %#v", payload.Errors)
	}
	if payload.DeploymentID != "deployment-1" {
		t.Fatalf("expected deployment id deployment-1, got %q", payload.DeploymentID)
	}
	if payload.JobsCreated != 2 {
		t.Fatalf("expected 2 jobs created, got %d", payload.JobsCreated)
	}
	if repository.jobsCreated != 2 {
		t.Fatalf("expected repository to create 2 jobs, got %d", repository.jobsCreated)
	}
}

func TestListAgentsMarksStaleHeartbeatUnhealthy(t *testing.T) {
	repository := &fakeDeploymentRepository{
		agents: []jobs.Agent{
			{
				ID:            "worker-1",
				Hostname:      "host-a",
				Status:        jobs.AgentStatusHealthy,
				LastHeartbeat: time.Now().Add(-jobs.AgentHeartbeatTimeout - time.Second),
				ActiveJobs:    1,
				CreatedAt:     time.Now(),
			},
		},
	}
	request := httptest.NewRequest(http.MethodGet, "/agents", nil)
	response := httptest.NewRecorder()

	listAgentsHandler(repository).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}

	var payload []jobs.Agent
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(payload))
	}
	if payload[0].Status != jobs.AgentStatusUnhealthy {
		t.Fatalf("expected unhealthy status, got %q", payload[0].Status)
	}
}

type fakeDeploymentRepository struct {
	deploymentID           string
	createDeploymentCalled bool
	jobsCreated            int
	agents                 []jobs.Agent
	deviceStates           map[string]jobs.DeviceState
}

func (r *fakeDeploymentRepository) CreateDeployment(ctx context.Context, rawConfig string) (string, error) {
	r.createDeploymentCalled = true
	if r.deploymentID == "" {
		return "deployment-1", nil
	}

	return r.deploymentID, nil
}

func (r *fakeDeploymentRepository) CreateJobsForDeployment(ctx context.Context, deploymentID string, devicesList []devices.Device) error {
	r.jobsCreated = len(devicesList)
	return nil
}

func (r *fakeDeploymentRepository) GetDeployments(ctx context.Context) ([]jobs.Deployment, error) {
	return nil, nil
}

func (r *fakeDeploymentRepository) GetDeployment(ctx context.Context, id string) (*jobs.Deployment, error) {
	return nil, nil
}

func (r *fakeDeploymentRepository) GetJobs(ctx context.Context) ([]jobs.Job, error) {
	return nil, nil
}

func (r *fakeDeploymentRepository) GetJobsByDeployment(ctx context.Context, deploymentID string) ([]jobs.Job, error) {
	return nil, nil
}

func (r *fakeDeploymentRepository) ListAgents(ctx context.Context) ([]jobs.Agent, error) {
	return r.agents, nil
}

func (r *fakeDeploymentRepository) GetDeviceState(ctx context.Context, deviceName string) (*jobs.DeviceState, error) {
	if r.deviceStates == nil {
		return nil, nil
	}

	state, ok := r.deviceStates[deviceName]
	if !ok {
		return nil, nil
	}

	return &state, nil
}

func (r *fakeDeploymentRepository) ListDeviceStates(ctx context.Context) ([]jobs.DeviceState, error) {
	if r.deviceStates == nil {
		return nil, nil
	}

	states := make([]jobs.DeviceState, 0, len(r.deviceStates))
	for _, state := range r.deviceStates {
		states = append(states, state)
	}

	return states, nil
}

func (r *fakeDeploymentRepository) UpsertDeviceState(ctx context.Context, deviceName string, deviceType string, actualConfig []byte) error {
	if r.deviceStates == nil {
		r.deviceStates = make(map[string]jobs.DeviceState)
	}

	existing := r.deviceStates[deviceName]
	r.deviceStates[deviceName] = jobs.DeviceState{
		DeviceName:   deviceName,
		DeviceType:   deviceType,
		ActualConfig: append([]byte(nil), actualConfig...),
		UpdatedAt:    existing.UpdatedAt,
	}
	return nil
}
