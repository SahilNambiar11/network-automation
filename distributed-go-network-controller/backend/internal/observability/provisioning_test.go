package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGrafanaProvisioningFilesExist(t *testing.T) {
	repoRoot := repositoryRoot(t)
	for _, relativePath := range []string{
		"grafana/provisioning/datasources/prometheus.yml",
		"grafana/provisioning/dashboards/dashboard.yml",
		"grafana/dashboards/network-controller.json",
	} {
		path := filepath.Join(repoRoot, relativePath)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", relativePath, err)
		}
	}
}

func TestGrafanaDashboardJSONIsValid(t *testing.T) {
	dashboardPath := filepath.Join(repositoryRoot(t), "grafana", "dashboards", "network-controller.json")
	data, err := os.ReadFile(dashboardPath)
	if err != nil {
		t.Fatalf("read dashboard JSON: %v", err)
	}

	var dashboard map[string]any
	if err := json.Unmarshal(data, &dashboard); err != nil {
		t.Fatalf("dashboard JSON is invalid: %v", err)
	}

	if dashboard["title"] != "Network Controller" {
		t.Fatalf("expected dashboard title Network Controller, got %#v", dashboard["title"])
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve current test file path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
}
