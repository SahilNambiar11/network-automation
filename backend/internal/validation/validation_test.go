package validation

import (
	"testing"

	"github.com/example/distributed-go-network-controller/backend/internal/devices"
)

func TestValidateNetworkConfigValid(t *testing.T) {
	response := ValidateNetworkConfig(validConfig())

	if !response.Valid {
		t.Fatalf("expected config to be valid, got errors: %#v", response.Errors)
	}
}

func TestValidateNetworkConfigDuplicateVLANIDFails(t *testing.T) {
	config := validConfig()
	config.VLANs[1].ID = config.VLANs[0].ID

	assertInvalidField(t, config, "vlans[1].id")
}

func TestValidateNetworkConfigInvalidCIDRFails(t *testing.T) {
	config := validConfig()
	config.VLANs[0].Subnet = "10.10.0.0"

	assertInvalidField(t, config, "vlans[0].subnet")
}

func TestValidateNetworkConfigOverlappingSubnetsFail(t *testing.T) {
	config := validConfig()
	config.VLANs[1].Subnet = "10.10.0.128/25"

	assertInvalidField(t, config, "vlans[1].subnet")
}

func TestValidateNetworkConfigInvalidFirewallPortFails(t *testing.T) {
	config := validConfig()
	config.FirewallRules[0].Port = 70000

	assertInvalidField(t, config, "firewall_rules[0].port")
}

func TestValidateNetworkConfigMissingVLANReferenceFails(t *testing.T) {
	config := validConfig()
	config.FirewallRules[0].Source = "unknown"

	assertInvalidField(t, config, "firewall_rules[0].source")
}

func TestValidateNetworkConfigInvalidFirewallActionFails(t *testing.T) {
	config := validConfig()
	config.FirewallRules[0].Action = "drop"

	assertInvalidField(t, config, "firewall_rules[0].action")
}

func TestValidateNetworkConfigDuplicateDeviceNameFails(t *testing.T) {
	config := validConfig()
	config.Devices[1].Name = config.Devices[0].Name

	assertInvalidField(t, config, "devices[1].name")
}

func validConfig() *devices.NetworkConfig {
	return &devices.NetworkConfig{
		Devices: []devices.Device{
			{Name: "core-router", Type: "router"},
			{Name: "access-switch", Type: "switch"},
		},
		VLANs: []devices.VLAN{
			{ID: 10, Name: "engineering", Subnet: "10.10.0.0/24"},
			{ID: 20, Name: "guest", Subnet: "10.20.0.0/24"},
		},
		FirewallRules: []devices.FirewallRule{
			{Source: "guest", Destination: "engineering", Port: 22, Action: "deny"},
		},
	}
}

func assertInvalidField(t *testing.T, config *devices.NetworkConfig, field string) {
	t.Helper()

	response := ValidateNetworkConfig(config)
	if response.Valid {
		t.Fatalf("expected config to be invalid")
	}

	for _, validationError := range response.Errors {
		if validationError.Field == field {
			return
		}
	}

	t.Fatalf("expected validation error for %q, got %#v", field, response.Errors)
}
