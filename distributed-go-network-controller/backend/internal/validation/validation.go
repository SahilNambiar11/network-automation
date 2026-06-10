package validation

import (
	"fmt"
	"net"

	"github.com/example/distributed-go-network-controller/backend/internal/devices"
)

const internet = "internet"

var allowedDeviceTypes = map[string]struct{}{
	"router":   {},
	"switch":   {},
	"firewall": {},
}

var allowedFirewallActions = map[string]struct{}{
	"allow": {},
	"deny":  {},
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationResponse struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

func ValidateNetworkConfig(config *devices.NetworkConfig) ValidationResponse {
	if config == nil {
		return ValidationResponse{
			Valid: false,
			Errors: []ValidationError{
				{Field: "config", Message: "network config is required"},
			},
		}
	}

	var errors []ValidationError

	errors = append(errors, validateDevices(config.Devices)...)
	vlanErrors, vlanNames, subnets := validateVLANs(config.VLANs)
	errors = append(errors, vlanErrors...)
	errors = append(errors, validateFirewallRules(config.FirewallRules, vlanNames)...)
	errors = append(errors, validateSubnetOverlap(subnets)...)

	return ValidationResponse{
		Valid:  len(errors) == 0,
		Errors: errors,
	}
}

func validateDevices(devicesList []devices.Device) []ValidationError {
	var errors []ValidationError

	if len(devicesList) == 0 {
		errors = append(errors, ValidationError{Field: "devices", Message: "at least one device is required"})
	}

	names := make(map[string]int)
	for index, device := range devicesList {
		if device.Name == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("devices[%d].name", index),
				Message: "device name cannot be empty",
			})
		} else if firstIndex, exists := names[device.Name]; exists {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("devices[%d].name", index),
				Message: fmt.Sprintf("duplicate device name also used at devices[%d].name", firstIndex),
			})
		} else {
			names[device.Name] = index
		}

		if _, ok := allowedDeviceTypes[device.Type]; !ok {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("devices[%d].type", index),
				Message: "device type must be router, switch, or firewall",
			})
		}
	}

	return errors
}

type parsedSubnet struct {
	field  string
	subnet *net.IPNet
}

func validateVLANs(vlans []devices.VLAN) ([]ValidationError, map[string]struct{}, []parsedSubnet) {
	var errors []ValidationError
	vlanIDs := make(map[int]int)
	vlanNames := make(map[string]struct{})
	vlanNameIndexes := make(map[string]int)
	subnets := make([]parsedSubnet, 0, len(vlans))

	if len(vlans) == 0 {
		errors = append(errors, ValidationError{Field: "vlans", Message: "at least one VLAN is required"})
	}

	for index, vlan := range vlans {
		if vlan.ID < 1 || vlan.ID > 4094 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("vlans[%d].id", index),
				Message: "VLAN ID must be between 1 and 4094",
			})
		} else if firstIndex, exists := vlanIDs[vlan.ID]; exists {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("vlans[%d].id", index),
				Message: fmt.Sprintf("duplicate VLAN ID also used at vlans[%d].id", firstIndex),
			})
		} else {
			vlanIDs[vlan.ID] = index
		}

		if vlan.Name == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("vlans[%d].name", index),
				Message: "VLAN name cannot be empty",
			})
		} else if firstIndex, exists := vlanNameIndexes[vlan.Name]; exists {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("vlans[%d].name", index),
				Message: fmt.Sprintf("duplicate VLAN name also used at vlans[%d].name", firstIndex),
			})
		} else {
			vlanNames[vlan.Name] = struct{}{}
			vlanNameIndexes[vlan.Name] = index
		}

		_, subnet, err := net.ParseCIDR(vlan.Subnet)
		if err != nil {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("vlans[%d].subnet", index),
				Message: "invalid CIDR block",
			})
			continue
		}

		subnets = append(subnets, parsedSubnet{
			field:  fmt.Sprintf("vlans[%d].subnet", index),
			subnet: subnet,
		})
	}

	return errors, vlanNames, subnets
}

func validateFirewallRules(rules []devices.FirewallRule, vlanNames map[string]struct{}) []ValidationError {
	var errors []ValidationError

	for index, rule := range rules {
		if !isKnownNetworkReference(rule.Source, vlanNames) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("firewall_rules[%d].source", index),
				Message: "source must reference an existing VLAN name or internet",
			})
		}

		if !isKnownNetworkReference(rule.Destination, vlanNames) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("firewall_rules[%d].destination", index),
				Message: "destination must reference an existing VLAN name or internet",
			})
		}

		if rule.Port < 1 || rule.Port > 65535 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("firewall_rules[%d].port", index),
				Message: "port must be between 1 and 65535",
			})
		}

		if _, ok := allowedFirewallActions[rule.Action]; !ok {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("firewall_rules[%d].action", index),
				Message: "action must be allow or deny",
			})
		}
	}

	return errors
}

func validateSubnetOverlap(subnets []parsedSubnet) []ValidationError {
	var errors []ValidationError

	for i := 0; i < len(subnets); i++ {
		for j := i + 1; j < len(subnets); j++ {
			if subnetsOverlap(subnets[i].subnet, subnets[j].subnet) {
				errors = append(errors, ValidationError{
					Field:   subnets[j].field,
					Message: fmt.Sprintf("subnet overlaps with %s", subnets[i].field),
				})
			}
		}
	}

	return errors
}

func isKnownNetworkReference(reference string, vlanNames map[string]struct{}) bool {
	if reference == internet {
		return true
	}

	_, ok := vlanNames[reference]
	return ok
}

func subnetsOverlap(first, second *net.IPNet) bool {
	return first.Contains(second.IP) || second.Contains(first.IP)
}
