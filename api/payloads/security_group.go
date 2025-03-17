package payloads

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	jellidation "github.com/jellydator/validation"
)

type SecurityGroupRelationships struct {
	RunningSpaces ToManyRelationship `json:"running_spaces"`
	StagingSpaces ToManyRelationship `json:"staging_spaces"`
}

type SecurityGroupCreate struct {
	DisplayName     string                                `json:"name"`
	Rules           []korifiv1alpha1.SecurityGroupRule    `json:"rules"`
	GloballyEnabled korifiv1alpha1.SecurityGroupWorkloads `json:"globally_enabled"`
	Relationships   SecurityGroupRelationships            `json:"relationships"`
}

func (c SecurityGroupCreate) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.DisplayName, jellidation.Required),
		jellidation.Field(&c.Rules, jellidation.Required, jellidation.By(validateSecurityGroupRules)),
	)
}

func (c SecurityGroupCreate) ToMessage() repositories.CreateSecurityGroupMessage {
	spaces := make(map[string]korifiv1alpha1.SecurityGroupWorkloads)

	for _, guid := range c.Relationships.RunningSpaces.CollectGUIDs() {
		workloads := spaces[guid]
		workloads.Running = true
		spaces[guid] = workloads
	}

	for _, guid := range c.Relationships.StagingSpaces.CollectGUIDs() {
		workloads := spaces[guid]
		workloads.Staging = true
		spaces[guid] = workloads
	}

	return repositories.CreateSecurityGroupMessage{
		DisplayName:     c.DisplayName,
		Rules:           c.Rules,
		GloballyEnabled: c.GloballyEnabled,
		Spaces:          spaces,
	}
}

func validateSecurityGroupRules(value any) error {
	rules := value.([]korifiv1alpha1.SecurityGroupRule)

	for i, rule := range rules {
		if len(rule.Protocol) != 0 {
			if rule.Protocol != korifiv1alpha1.ProtocolALL && rule.Protocol != korifiv1alpha1.ProtocolTCP && rule.Protocol != korifiv1alpha1.ProtocolUDP {
				return fmt.Errorf("rules[%d]: protocol %s not supported", i, rule.Protocol)
			}
		}

		if rule.Protocol == korifiv1alpha1.ProtocolALL && len(rule.Ports) != 0 {
			return fmt.Errorf("rules[%d]: ports are not allowed for protocols of type all", i)
		}

		if (rule.Protocol == korifiv1alpha1.ProtocolTCP || rule.Protocol == korifiv1alpha1.ProtocolUDP) && len(rule.Ports) == 0 {
			return fmt.Errorf("rules[%d]: ports are required for protocols of type TCP and UDP, ports must be a valid single port, comma separated list of ports, or range of ports, formatted as a string", i)
		}

		if err := validateRuleDestination(rule.Destination); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}

		if err := validateRulePorts(rule.Ports); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
	}

	return nil
}

func validateRuleDestination(destination string) error {
	// Check for single IPv4 address
	if ip := net.ParseIP(destination); ip != nil && ip.To4() != nil {
		return nil
	}

	// Check for CIDR notation
	if ip, ipnet, err := net.ParseCIDR(destination); err == nil && ip.To4() != nil {
		ones, _ := ipnet.Mask.Size()
		if ones >= 1 && ones <= 32 {
			return nil
		}
	}

	// Check for IP range
	parts := strings.Split(destination, "-")
	if len(parts) == 2 {
		ip1 := net.ParseIP(parts[0])
		ip2 := net.ParseIP(parts[1])
		if ip1 != nil && ip1.To4() != nil && ip2 != nil && ip2.To4() != nil {
			return nil
		}
	}

	return fmt.Errorf("the destination: %s is not in a valid format", destination)
}

func validateRulePorts(ports string) error {
	if len(ports) == 0 {
		return nil
	}

	if strings.Count(ports, "-") == 1 && !strings.Contains(ports, ",") {
		// Port range
		parts := strings.Split(ports, "-")
		if len(parts) == 2 && isValidPort(parts[0]) && isValidPort(parts[1]) {
			return nil
		}
	} else {
		// Single port or comma-separated list
		parts := strings.Split(ports, ",")
		for _, part := range parts {
			if !isValidPort(part) {
				return fmt.Errorf("invalid port: %s", part)
			}
		}
		return nil
	}

	return fmt.Errorf("the ports: %s is not in a valid format", ports)
}

func isValidPort(portStr string) bool {
	if len(portStr) == 0 || portStr[0] == '0' {
		return false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return false
	}
	return port >= 1 && port <= 65535
}
