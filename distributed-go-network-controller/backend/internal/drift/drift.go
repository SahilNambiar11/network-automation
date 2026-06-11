package drift

import (
	"encoding/json"
	"sort"

	"github.com/example/distributed-go-network-controller/backend/internal/devices"
)

type DriftReport struct {
	DeviceName string `json:"device_name"`
	Drift      bool   `json:"drift"`

	MissingVLANs []int `json:"missing_vlans,omitempty"`
	ExtraVLANs   []int `json:"extra_vlans,omitempty"`

	MissingFirewallRules []devices.FirewallRule `json:"missing_firewall_rules,omitempty"`
	ExtraFirewallRules   []devices.FirewallRule `json:"extra_firewall_rules,omitempty"`
}

type DriftSummary struct {
	DevicesChecked   int `json:"devices_checked"`
	DevicesWithDrift int `json:"devices_with_drift"`
}

func GenerateDriftReport(desired *devices.NetworkConfig, actualConfig map[string]any, deviceName string) DriftReport {
	report := DriftReport{DeviceName: deviceName}

	actualVLANs := actualVLANIDs(actualConfig["vlans"])
	desiredVLANs := desiredVLANIDs(desired.VLANs)

	for vlanID := range desiredVLANs {
		if !actualVLANs[vlanID] {
			report.MissingVLANs = append(report.MissingVLANs, vlanID)
		}
	}
	for vlanID := range actualVLANs {
		if !desiredVLANs[vlanID] {
			report.ExtraVLANs = append(report.ExtraVLANs, vlanID)
		}
	}

	actualRules := actualFirewallRules(actualConfig["firewall_rules"])
	report.MissingFirewallRules, report.ExtraFirewallRules = compareFirewallRules(desired.FirewallRules, actualRules)
	sort.Ints(report.MissingVLANs)
	sort.Ints(report.ExtraVLANs)
	sortFirewallRules(report.MissingFirewallRules)
	sortFirewallRules(report.ExtraFirewallRules)
	report.Drift = len(report.MissingVLANs) > 0 ||
		len(report.ExtraVLANs) > 0 ||
		len(report.MissingFirewallRules) > 0 ||
		len(report.ExtraFirewallRules) > 0

	return report
}

func SummarizeReports(reports []DriftReport) DriftSummary {
	summary := DriftSummary{DevicesChecked: len(reports)}
	for _, report := range reports {
		if report.Drift {
			summary.DevicesWithDrift++
		}
	}

	return summary
}

func desiredVLANIDs(vlans []devices.VLAN) map[int]bool {
	ids := make(map[int]bool, len(vlans))
	for _, vlan := range vlans {
		ids[vlan.ID] = true
	}

	return ids
}

func actualVLANIDs(value any) map[int]bool {
	vlans := make(map[int]bool)
	rawVLANs, ok := value.([]any)
	if !ok {
		return vlans
	}

	for _, rawVLAN := range rawVLANs {
		fields, ok := rawVLAN.(map[string]any)
		if !ok {
			continue
		}

		switch id := fields["id"].(type) {
		case float64:
			vlans[int(id)] = true
		case int:
			vlans[id] = true
		}
	}

	return vlans
}

func actualFirewallRules(value any) []devices.FirewallRule {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}

	var rules []devices.FirewallRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil
	}

	return rules
}

func compareFirewallRules(desiredRules []devices.FirewallRule, actualRules []devices.FirewallRule) ([]devices.FirewallRule, []devices.FirewallRule) {
	desiredCounts := firewallRuleCounts(desiredRules)
	actualCounts := firewallRuleCounts(actualRules)

	var missingRules []devices.FirewallRule
	for rule, desiredCount := range desiredCounts {
		for count := actualCounts[rule]; count < desiredCount; count++ {
			missingRules = append(missingRules, rule)
		}
	}

	var extraRules []devices.FirewallRule
	for rule, actualCount := range actualCounts {
		for count := desiredCounts[rule]; count < actualCount; count++ {
			extraRules = append(extraRules, rule)
		}
	}

	return missingRules, extraRules
}

func firewallRuleCounts(rules []devices.FirewallRule) map[devices.FirewallRule]int {
	counts := make(map[devices.FirewallRule]int, len(rules))
	for _, rule := range rules {
		counts[rule]++
	}

	return counts
}

func sortFirewallRules(rules []devices.FirewallRule) {
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Source != rules[j].Source {
			return rules[i].Source < rules[j].Source
		}
		if rules[i].Destination != rules[j].Destination {
			return rules[i].Destination < rules[j].Destination
		}
		if rules[i].Port != rules[j].Port {
			return rules[i].Port < rules[j].Port
		}

		return rules[i].Action < rules[j].Action
	})
}
