package devices

import "gopkg.in/yaml.v3"

type NetworkConfig struct {
	Devices       []Device       `yaml:"devices" json:"devices"`
	VLANs         []VLAN         `yaml:"vlans" json:"vlans"`
	FirewallRules []FirewallRule `yaml:"firewall_rules" json:"firewall_rules"`
}

type Device struct {
	Name string `yaml:"name" json:"name"`
	Type string `yaml:"type" json:"type"`
}

type VLAN struct {
	ID     int    `yaml:"id" json:"id"`
	Name   string `yaml:"name" json:"name"`
	Subnet string `yaml:"subnet" json:"subnet"`
}

type FirewallRule struct {
	Source      string `yaml:"source" json:"source"`
	Destination string `yaml:"destination" json:"destination"`
	Port        int    `yaml:"port" json:"port"`
	Action      string `yaml:"action" json:"action"`
}

func ParseYAML(data []byte) (*NetworkConfig, error) {
	var config NetworkConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
