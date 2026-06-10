package api

import (
	"encoding/json"
	"net/http"
)

type mutateDeviceRequest struct {
	RemoveVLAN         *int `json:"remove_vlan"`
	ClearFirewallRules bool `json:"clear_firewall_rules"`
}

func listDevicesHandler(repository DeploymentRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		devices, err := repository.ListDeviceStates(r.Context())
		if err != nil {
			http.Error(w, "failed to get devices", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, devices)
	}
}

func getDeviceHandler(repository DeploymentRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		device, err := repository.GetDeviceState(r.Context(), r.PathValue("name"))
		if err != nil {
			http.Error(w, "failed to get device", http.StatusInternalServerError)
			return
		}
		if device == nil {
			http.NotFound(w, r)
			return
		}

		writeJSON(w, http.StatusOK, device)
	}
}

func mutateDeviceHandler(repository DeploymentRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deviceName := r.PathValue("name")
		device, err := repository.GetDeviceState(r.Context(), deviceName)
		if err != nil {
			http.Error(w, "failed to get device", http.StatusInternalServerError)
			return
		}
		if device == nil {
			http.NotFound(w, r)
			return
		}

		var request mutateDeviceRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid mutation request", http.StatusBadRequest)
			return
		}
		if request.RemoveVLAN == nil && !request.ClearFirewallRules {
			http.Error(w, "unsupported mutation", http.StatusBadRequest)
			return
		}

		var actualConfig map[string]any
		if err := json.Unmarshal(device.ActualConfig, &actualConfig); err != nil {
			http.Error(w, "failed to parse device state", http.StatusInternalServerError)
			return
		}

		if request.RemoveVLAN != nil {
			actualConfig["vlans"] = removeVLAN(actualConfig["vlans"], *request.RemoveVLAN)
		}
		if request.ClearFirewallRules {
			actualConfig["firewall_rules"] = []any{}
		}

		updatedConfig, err := json.Marshal(actualConfig)
		if err != nil {
			http.Error(w, "failed to encode device state", http.StatusInternalServerError)
			return
		}

		if err := repository.UpsertDeviceState(r.Context(), device.DeviceName, device.DeviceType, updatedConfig); err != nil {
			http.Error(w, "failed to update device state", http.StatusInternalServerError)
			return
		}

		updatedDevice, err := repository.GetDeviceState(r.Context(), deviceName)
		if err != nil {
			http.Error(w, "failed to get updated device", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, updatedDevice)
	}
}

func removeVLAN(value any, vlanID int) []any {
	vlans, ok := value.([]any)
	if !ok {
		return []any{}
	}

	filtered := make([]any, 0, len(vlans))
	for _, vlan := range vlans {
		fields, ok := vlan.(map[string]any)
		if !ok {
			filtered = append(filtered, vlan)
			continue
		}

		id, ok := fields["id"].(float64)
		if !ok || int(id) != vlanID {
			filtered = append(filtered, vlan)
		}
	}

	return filtered
}
