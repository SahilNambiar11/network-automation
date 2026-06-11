package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/distributed-go-network-controller/backend/internal/jobs"
)

func TestListDevicesReturnsDeviceStates(t *testing.T) {
	repository := &fakeDeploymentRepository{
		deviceStates: map[string]jobs.DeviceState{
			"core-router": testDeviceState(),
		},
	}
	request := httptest.NewRequest(http.MethodGet, "/devices", nil)
	response := httptest.NewRecorder()

	listDevicesHandler(repository).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}

	var payload []jobs.DeviceState
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1 device, got %d", len(payload))
	}
	if payload[0].DeviceName != "core-router" {
		t.Fatalf("expected core-router, got %q", payload[0].DeviceName)
	}
}

func TestGetDeviceReturnsDeviceState(t *testing.T) {
	repository := &fakeDeploymentRepository{
		deviceStates: map[string]jobs.DeviceState{
			"core-router": testDeviceState(),
		},
	}
	request := httptest.NewRequest(http.MethodGet, "/devices/core-router", nil)
	request.SetPathValue("name", "core-router")
	response := httptest.NewRecorder()

	getDeviceHandler(repository).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}

	var payload jobs.DeviceState
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.DeviceName != "core-router" {
		t.Fatalf("expected core-router, got %q", payload.DeviceName)
	}
}

func TestGetDeviceNotFound(t *testing.T) {
	repository := &fakeDeploymentRepository{}
	request := httptest.NewRequest(http.MethodGet, "/devices/missing", nil)
	request.SetPathValue("name", "missing")
	response := httptest.NewRecorder()

	getDeviceHandler(repository).ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.Code)
	}
}

func TestMutateDeviceRemoveVLAN(t *testing.T) {
	repository := &fakeDeploymentRepository{
		deviceStates: map[string]jobs.DeviceState{
			"core-router": testDeviceState(),
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/devices/core-router/mutate", bytes.NewBufferString(`{"remove_vlan":10}`))
	request.SetPathValue("name", "core-router")
	response := httptest.NewRecorder()

	mutateDeviceHandler(repository).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}

	var actualConfig struct {
		VLANs []struct {
			ID int `json:"id"`
		} `json:"vlans"`
	}
	if err := json.Unmarshal(repository.deviceStates["core-router"].ActualConfig, &actualConfig); err != nil {
		t.Fatalf("decode actual config: %v", err)
	}
	if len(actualConfig.VLANs) != 1 || actualConfig.VLANs[0].ID != 20 {
		t.Fatalf("expected only VLAN 20 after mutation, got %#v", actualConfig.VLANs)
	}
}

func TestMutateDeviceClearFirewallRules(t *testing.T) {
	repository := &fakeDeploymentRepository{
		deviceStates: map[string]jobs.DeviceState{
			"core-router": testDeviceState(),
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/devices/core-router/mutate", bytes.NewBufferString(`{"clear_firewall_rules":true}`))
	request.SetPathValue("name", "core-router")
	response := httptest.NewRecorder()

	mutateDeviceHandler(repository).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}

	var actualConfig struct {
		FirewallRules []any `json:"firewall_rules"`
	}
	if err := json.Unmarshal(repository.deviceStates["core-router"].ActualConfig, &actualConfig); err != nil {
		t.Fatalf("decode actual config: %v", err)
	}
	if len(actualConfig.FirewallRules) != 0 {
		t.Fatalf("expected firewall rules to be cleared, got %#v", actualConfig.FirewallRules)
	}
}

func testDeviceState() jobs.DeviceState {
	return jobs.DeviceState{
		DeviceName: "core-router",
		DeviceType: "router",
		ActualConfig: json.RawMessage(`{
			"vlans": [
				{"id": 10, "name": "engineering", "subnet": "10.10.0.0/24"},
				{"id": 20, "name": "guest", "subnet": "10.20.0.0/24"}
			],
			"firewall_rules": [
				{"source": "guest", "destination": "engineering", "port": 22, "action": "deny"}
			],
			"last_deployment_id": "deployment-1",
			"last_job_id": "job-1"
		}`),
		UpdatedAt: time.Now(),
	}
}
