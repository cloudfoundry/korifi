package security_groups

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	validationwebhook "code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	SecurityGroupEntityType           = "securityGroup"
	InvalidSecurityGroupRuleErrorType = "InvalidSecurityGroupRuleError"
	InvalidPortsErrorMessage          = "ports must be a valid single port, comma separated list of ports, or range or ports, formatted as a string"
	InvalidDestinationErrorMessage    = "destination must contain valid CIDR(s), IP address(es), or IP address range(s)"
	InvalidNameErrorMessage           = "display name cannot be empty and must be less than 255 characters"
)

var cfsecuritygrouplog = logf.Log.WithName("cfsecuritygroup-validation")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfsecuritygroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfsecuritygroups,verbs=create;update;delete,versions=v1alpha1,name=vcfsecuritygroup.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type Validator struct {
	duplicateValidator webhooks.NameValidator
	rootNamespace      string
}

var _ admission.Validator[*korifiv1alpha1.CFSecurityGroup] = &Validator{}

func NewValidator(duplicateValidator webhooks.NameValidator, rootNamespace string) *Validator {
	return &Validator{
		duplicateValidator: duplicateValidator,
		rootNamespace:      rootNamespace,
	}
}

func (v *Validator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &korifiv1alpha1.CFSecurityGroup{}).
		WithValidator(v).
		Complete()
}

func (v *Validator) ValidateCreate(ctx context.Context, securityGroup *korifiv1alpha1.CFSecurityGroup) (admission.Warnings, error) {
	return nil, v.duplicateValidator.ValidateCreate(ctx, cfsecuritygrouplog, v.rootNamespace, securityGroup)
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldSecurityGroup, securityGroup *korifiv1alpha1.CFSecurityGroup) (admission.Warnings, error) {
	if !securityGroup.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

	if err := v.validateSecurityGroup(ctx, securityGroup); err != nil {
		return nil, err
	}

	return nil, v.duplicateValidator.ValidateUpdate(ctx, cfsecuritygrouplog, v.rootNamespace, oldSecurityGroup, securityGroup)
}

func (v *Validator) ValidateDelete(ctx context.Context, securityGroup *korifiv1alpha1.CFSecurityGroup) (admission.Warnings, error) {
	return nil, v.duplicateValidator.ValidateDelete(ctx, cfsecuritygrouplog, v.rootNamespace, securityGroup)
}

func (v *Validator) validateSecurityGroup(ctx context.Context, secGroup *korifiv1alpha1.CFSecurityGroup) error {
	if err := validateName(secGroup.Spec.DisplayName); err != nil {
		return validationwebhook.ValidationError{
			Type:    InvalidNameErrorMessage,
			Message: err.Error(),
		}.ExportJSONError()
	}

	if err := v.validateRules(secGroup.Spec.Rules); err != nil {
		return validationwebhook.ValidationError{
			Type:    InvalidSecurityGroupRuleErrorType,
			Message: err.Error(),
		}.ExportJSONError()
	}
	return nil
}

func validateName(name string) error {
	if len(name) == 0 || len(name) > 255 {
		return errors.New(InvalidNameErrorMessage)
	}
	return nil
}

func (v *Validator) validateRules(rules []korifiv1alpha1.SecurityGroupRule) error {
	for i, rule := range rules {
		destinations := strings.SplitSeq(rule.Destination, ",")
		for dest := range destinations {
			if err := validateRuleDestination(strings.TrimSpace(dest)); err != nil {
				return fmt.Errorf("rules[%d]: %w", i, err)
			}
		}

		if err := validateRulePorts(rule.Ports, rule.Protocol); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
		if err := validateRuleICMP(rule.Type, rule.Code, rule.Protocol); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
	}
	return nil
}

func validateRuleDestination(destination string) error {
	if ip := net.ParseIP(destination); ip != nil && ip.To4() != nil {
		return nil
	}
	if ip, ipnet, err := net.ParseCIDR(destination); err == nil && ip.To4() != nil {
		ones, _ := ipnet.Mask.Size()
		if ones >= 1 && ones <= 32 {
			return nil
		}
	}
	parts := strings.Split(destination, "-")
	if len(parts) == 2 {
		if net.ParseIP(parts[0]).To4() == nil || net.ParseIP(parts[1]).To4() == nil {
			return errors.New("destination IP address range is invalid")
		}
		return nil
	}
	return errors.New(InvalidDestinationErrorMessage)
}

func validateRuleICMP(icmpType, icmpCode int, protocol string) error {
	if protocol == korifiv1alpha1.ProtocolICMP {
		return nil
	}

	if protocol == korifiv1alpha1.ProtocolICMPv6 {
		return nil
	}

	if icmpType != 0 {
		return errors.New("type allowed for ICMP and ICMPv6 only")
	}

	if icmpCode != 0 {
		return errors.New("code allowed for ICMP and ICMPv6 only")
	}

	return nil
}

func validateRulePorts(ports, protocol string) error {
	if protocol == korifiv1alpha1.ProtocolALL {
		if ports != "" {
			return errors.New("ports are not allowed for protocols of type all")
		}
		return nil
	}

	if protocol == korifiv1alpha1.ProtocolTCP || protocol == korifiv1alpha1.ProtocolUDP {
		if ports == "" {
			return fmt.Errorf("ports are required for protocols of type TCP and UDP, %s", InvalidPortsErrorMessage)
		}

		portRange := strings.Split(ports, "-")
		portValues := strings.Split(ports, ",")

		if len(portRange) > 1 && len(portValues) > 1 {
			return errors.New(InvalidPortsErrorMessage)
		}

		if len(portRange) == 2 {
			parts := portRange
			if isValidPort(parts[0]) && isValidPort(parts[1]) {
				return nil
			}
		}

		parts := portValues
		for _, part := range parts {
			if !isValidPort(part) {
				return errors.New(InvalidPortsErrorMessage)
			}
		}
	}
	return nil
}

func isValidPort(portStr string) bool {
	port, err := strconv.Atoi(portStr)
	return err == nil && port >= 1 && port <= 65535
}
