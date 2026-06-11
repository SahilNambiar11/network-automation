package drift

import (
	"testing"

	"github.com/example/distributed-go-network-controller/backend/internal/devices"
)

func TestGenerateDriftReportNoDrift(t *testing.T) {
	report := GenerateDriftReport(desiredConfig(), actualConfig(), "core-router")

	if report.Drift {
		t.Fatalf("expected no drift, got %#v", report)
	}
}

func TestGenerateDriftReportMissingVLAN(t *testing.T) {
	actual := actualConfig()
	actual["vlans"] = []any{
		map[string]any{"id": float64(20), "name": "guest", "subnet": "10.20.0.0/24"},
	}

	report := GenerateDriftReport(desiredConfig(), actual, "core-router")

	if !report.Drift {
		t.Fatalf("expected drift")
	}
	if len(report.MissingVLANs) != 1 || report.MissingVLANs[0] != 10 {
		t.Fatalf("expected missing VLAN 10, got %#v", report.MissingVLANs)
	}
}

func TestGenerateDriftReportExtraVLAN(t *testing.T) {
	actual := actualConfig()
	actual["vlans"] = []any{
		map[string]any{"id": float64(10), "name": "engineering", "subnet": "10.10.0.0/24"},
		map[string]any{"id": float64(20), "name": "guest", "subnet": "10.20.0.0/24"},
		map[string]any{"id": float64(30), "name": "lab", "subnet": "10.30.0.0/24"},
	}

	report := GenerateDriftReport(desiredConfig(), actual, "core-router")

	if !report.Drift {
		t.Fatalf("expected drift")
	}
	if len(report.ExtraVLANs) != 1 || report.ExtraVLANs[0] != 30 {
		t.Fatalf("expected extra VLAN 30, got %#v", report.ExtraVLANs)
	}
}

func TestGenerateDriftReportMissingFirewallRule(t *testing.T) {
	actual := actualConfig()
	actual["firewall_rules"] = []any{}

	report := GenerateDriftReport(desiredConfig(), actual, "core-router")

	if !report.Drift {
		t.Fatalf("expected drift")
	}
	if len(report.MissingFirewallRules) != 1 {
		t.Fatalf("expected one missing firewall rule, got %#v", report.MissingFirewallRules)
	}
	if report.MissingFirewallRules[0].Destination != "engineering" {
		t.Fatalf("expected missing rule for engineering, got %#v", report.MissingFirewallRules[0])
	}
}

func TestGenerateDriftReportExtraFirewallRule(t *testing.T) {
	actual := actualConfig()
	actual["firewall_rules"] = []any{
		map[string]any{"source": "guest", "destination": "engineering", "port": float64(22), "action": "deny"},
		map[string]any{"source": "internet", "destination": "guest", "port": float64(443), "action": "allow"},
	}

	report := GenerateDriftReport(desiredConfig(), actual, "core-router")

	if !report.Drift {
		t.Fatalf("expected drift")
	}
	if len(report.ExtraFirewallRules) != 1 {
		t.Fatalf("expected one extra firewall rule, got %#v", report.ExtraFirewallRules)
	}
	if report.ExtraFirewallRules[0].Source != "internet" {
		t.Fatalf("expected extra internet rule, got %#v", report.ExtraFirewallRules[0])
	}
}

func TestSummarizeReports(t *testing.T) {
	summary := SummarizeReports([]DriftReport{
		{DeviceName: "core-router", Drift: true},
		{DeviceName: "access-switch", Drift: false},
	})

	if summary.DevicesChecked != 2 {
		t.Fatalf("expected 2 devices checked, got %d", summary.DevicesChecked)
	}
	if summary.DevicesWithDrift != 1 {
		t.Fatalf("expected 1 device with drift, got %d", summary.DevicesWithDrift)
	}
}

func desiredConfig() *devices.NetworkConfig {
	return &devices.NetworkConfig{
		VLANs: []devices.VLAN{
			{ID: 10, Name: "engineering", Subnet: "10.10.0.0/24"},
			{ID: 20, Name: "guest", Subnet: "10.20.0.0/24"},
		},
		FirewallRules: []devices.FirewallRule{
			{Source: "guest", Destination: "engineering", Port: 22, Action: "deny"},
		},
	}
}

func actualConfig() map[string]any {
	return map[string]any{
		"vlans": []any{
			map[string]any{"id": float64(10), "name": "engineering", "subnet": "10.10.0.0/24"},
			map[string]any{"id": float64(20), "name": "guest", "subnet": "10.20.0.0/24"},
		},
		"firewall_rules": []any{
			map[string]any{"source": "guest", "destination": "engineering", "port": float64(22), "action": "deny"},
		},
		"last_deployment_id": "deployment-1",
		"last_job_id":        "job-1",
	}
}
