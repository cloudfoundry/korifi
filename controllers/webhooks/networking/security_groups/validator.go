package security_groups

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	validationwebhook "code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	SecurityGroupEntityType           = "securityGroup"
	InvalidSecurityGroupRuleErrorType = "InvalidSecurityGroupRuleError"
	InvalidPortsErrorMessage          = "ports must be a valid single port, comma separated list of ports, or range or ports, formatted as a string"
	InvalidDestinationErrorMessage    = "destination must contain valid CIDR(s), IP address(es), or IP address range(s)"
)

var cfsecuritygrouplog = logf.Log.WithName("cfsecuritygroup-validation")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfsecuritygroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfsecuritygroups,verbs=create;update;delete,versions=v1alpha1,name=vcfsecuritygroup.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type Validator struct {
	duplicateValidator webhooks.NameValidator
	rootNamespace      string
}

var _ webhook.CustomValidator = &Validator{}

func NewValidator(duplicateValidator webhooks.NameValidator, rootNamespace string) *Validator {
	return &Validator{
		duplicateValidator: duplicateValidator,
		rootNamespace:      rootNamespace,
	}
}

func (v *Validator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&korifiv1alpha1.CFSecurityGroup{}).
		WithValidator(v).
		Complete()
}

func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	securityGroup, ok := obj.(*korifiv1alpha1.CFSecurityGroup)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFSecurityGroup but got a %T", obj))
	}

	if err := v.validateRules(securityGroup.Spec.Rules); err != nil {
		return nil, validationwebhook.ValidationError{
			Type:    InvalidSecurityGroupRuleErrorType,
			Message: err.Error(),
		}.ExportJSONError()
	}

	return nil, v.duplicateValidator.ValidateCreate(ctx, cfsecuritygrouplog, v.rootNamespace, securityGroup)
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) (admission.Warnings, error) {
	securityGroup, ok := obj.(*korifiv1alpha1.CFSecurityGroup)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFSecurityGroup but got a %T", obj))
	}

	if !securityGroup.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

	if err := v.validateRules(securityGroup.Spec.Rules); err != nil {
		return nil, validationwebhook.ValidationError{
			Type:    InvalidSecurityGroupRuleErrorType,
			Message: err.Error(),
		}
	}

	oldSecurityGroup, ok := oldObj.(*korifiv1alpha1.CFSecurityGroup)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFSecurityGroup but got a %T", oldObj))
	}

	return nil, v.duplicateValidator.ValidateUpdate(ctx, cfsecuritygrouplog, v.rootNamespace, oldSecurityGroup, securityGroup)
}

func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	securityGroup, ok := obj.(*korifiv1alpha1.CFSecurityGroup)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFSecurityGroup but got a %T", obj))
	}

	return nil, v.duplicateValidator.ValidateDelete(ctx, cfsecuritygrouplog, v.rootNamespace, securityGroup)
}

func (v *Validator) validateRules(rules []korifiv1alpha1.SecurityGroupRule) error {
	for i, rule := range rules {
		if err := validateRuleDestination(rule.Destination); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
		if err := validateRulePorts(rule.Ports, rule.Protocol); err != nil {
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
			return fmt.Errorf("destination IP address range is invalid")
		}
		return nil
	}
	return errors.New(InvalidDestinationErrorMessage)
}

func validateRulePorts(ports, protocol string) error {
	if !slices.Contains([]string{"tcp", "udp", "all"}, protocol) {
		return fmt.Errorf("protocol must be 'tcp', 'udp', or 'all'")
	}

	if protocol == korifiv1alpha1.ProtocolALL {
		if ports != "" {
			return fmt.Errorf("ports are not allowed for protocols of type all")
		}
		return nil
	}

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
	return nil
}

func isValidPort(portStr string) bool {
	port, err := strconv.Atoi(portStr)
	return err == nil && port >= 1 && port <= 65535
}
