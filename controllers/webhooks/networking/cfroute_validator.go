package networking

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"github.com/hashicorp/go-multierror"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	RouteEntityType = "route"

	RouteDestinationNotInSpaceErrorType    = "RouteDestinationNotInSpaceError"
	RouteDestinationNotInSpaceErrorMessage = "Route destination app not found in space"
	RouteHostNameValidationErrorType       = "RouteHostNameValidationError"
	RoutePathValidationErrorType           = "RoutePathValidationError"
	RouteSubdomainValidationErrorType      = "RouteSubdomainValidationError"
	RouteSubdomainValidationErrorMessage   = "Subdomains must each be at most 63 characters"

	HostEmptyError  = "host cannot be empty"
	HostLengthError = "host is too long (maximum is 63 characters)"
	HostFormatError = "host must be either \"*\" or contain only alphanumeric characters, \"_\", or \"-\""

	InvalidURIError          = "Invalid Route URI"
	PathIsSlashError         = "Path cannot be a single slash"
	PathHasQuestionMarkError = "Path cannot contain a question mark"
	PathLengthExceededError  = "Path cannot exceed 128 characters"
)

var logger = logf.Log.WithName("route-validation")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfroute,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfroutes,verbs=create;update;delete,versions=v1alpha1,name=vcfroute.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFRouteValidator struct {
	duplicateValidator webhooks.NameValidator
	rootNamespace      string
	client             client.Client
}

var _ webhook.CustomValidator = &CFRouteValidator{}

func NewCFRouteValidator(
	nameValidator webhooks.NameValidator,
	rootNamespace string,
	client client.Client,
) *CFRouteValidator {
	return &CFRouteValidator{
		duplicateValidator: nameValidator,
		rootNamespace:      rootNamespace,
		client:             client,
	}
}

func (v *CFRouteValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&korifiv1alpha1.CFRoute{}).
		WithValidator(v).
		Complete()
}

func (v *CFRouteValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	route, ok := obj.(*korifiv1alpha1.CFRoute)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFRoute but got a %T", obj))
	}

	domain, err := v.validateRoute(ctx, route)
	if err != nil {
		return err
	}

	duplicateErrorMessage := generateDuplicateErrorMessage(route, domain)
	validationErr := v.duplicateValidator.ValidateCreate(ctx, logger, v.rootNamespace, uniqueName(*route), duplicateErrorMessage)
	if validationErr != nil {
		return validationErr.ExportJSONError()
	}

	return nil
}

func (v *CFRouteValidator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) error {
	route, ok := obj.(*korifiv1alpha1.CFRoute)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFRoute but got a %T", obj))
	}

	if !route.GetDeletionTimestamp().IsZero() {
		return nil
	}

	oldRoute, ok := oldObj.(*korifiv1alpha1.CFRoute)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFRoute but got a %T", obj))
	}

	immutableError := webhooks.ValidationError{
		Type: webhooks.ImmutableFieldErrorType,
	}

	if route.Spec.Host != oldRoute.Spec.Host {
		immutableError.Message = fmt.Sprintf(webhooks.ImmutableFieldErrorMessageTemplate, "CFRoute.Spec.Host")
		return immutableError.ExportJSONError()
	}

	if route.Spec.Path != oldRoute.Spec.Path {
		immutableError.Message = fmt.Sprintf(webhooks.ImmutableFieldErrorMessageTemplate, "CFRoute.Spec.Path")
		return immutableError.ExportJSONError()
	}

	if route.Spec.Protocol != oldRoute.Spec.Protocol {
		immutableError.Message = fmt.Sprintf(webhooks.ImmutableFieldErrorMessageTemplate, "CFRoute.Spec.Protocol")
		return immutableError.ExportJSONError()
	}

	if route.Spec.DomainRef.Name != oldRoute.Spec.DomainRef.Name {
		immutableError.Message = fmt.Sprintf(webhooks.ImmutableFieldErrorMessageTemplate, "CFRoute.Spec.DomainRef.Name")
		return immutableError.ExportJSONError()
	}

	domain, err := v.validateDestinations(ctx, route)
	if err != nil {
		return err
	}

	duplicateErrorMessage := generateDuplicateErrorMessage(route, domain)
	validationErr := v.duplicateValidator.ValidateUpdate(ctx, logger, v.rootNamespace, uniqueName(*oldRoute), uniqueName(*route), duplicateErrorMessage)
	if validationErr != nil {
		return validationErr.ExportJSONError()
	}

	return nil
}

func (v *CFRouteValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	route, ok := obj.(*korifiv1alpha1.CFRoute)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFRoute but got a %T", obj))
	}

	validationErr := v.duplicateValidator.ValidateDelete(ctx, logger, v.rootNamespace, uniqueName(*route))
	if validationErr != nil {
		return validationErr.ExportJSONError()
	}

	return nil
}

func (v *CFRouteValidator) validateRoute(ctx context.Context, route *korifiv1alpha1.CFRoute) (*korifiv1alpha1.CFDomain, error) {
	domain, err := v.validateDestinations(ctx, route)
	if err != nil {
		return domain, err
	}

	if err = validateFQDN(route.Spec.Host, domain.Spec.Name); err != nil {
		return nil, err
	}

	if err = validatePath(route.Spec.Path); err != nil {
		return nil, err
	}

	return domain, nil
}

func (v *CFRouteValidator) fetchDomain(ctx context.Context, route *korifiv1alpha1.CFRoute) (*korifiv1alpha1.CFDomain, error) {
	domain := &korifiv1alpha1.CFDomain{}
	err := v.client.Get(ctx, types.NamespacedName{Name: route.Spec.DomainRef.Name, Namespace: route.Spec.DomainRef.Namespace}, domain)
	if err != nil {
		errMessage := "Error while retrieving CFDomain object"
		logger.Error(err, errMessage)
		return nil, webhooks.ValidationError{
			Type:    webhooks.UnknownErrorType,
			Message: errMessage,
		}.ExportJSONError()
	}
	return domain, err
}

func (v *CFRouteValidator) validateDestinations(ctx context.Context, route *korifiv1alpha1.CFRoute) (*korifiv1alpha1.CFDomain, error) {
	domain, err := v.fetchDomain(ctx, route)
	if err != nil {
		return domain, err
	}
	if err = v.checkDestinationsExistInNamespace(ctx, *route); err != nil {
		validationErr := webhooks.ValidationError{}

		if apierrors.IsNotFound(err) {
			validationErr.Type = RouteDestinationNotInSpaceErrorType
			validationErr.Message = RouteDestinationNotInSpaceErrorMessage
		} else {
			validationErr.Type = webhooks.UnknownErrorType
			validationErr.Message = webhooks.UnknownErrorMessage
		}

		logger.Error(err, validationErr.Message)
		return domain, validationErr.ExportJSONError()
	}
	return domain, nil
}

func generateDuplicateErrorMessage(route *korifiv1alpha1.CFRoute, domain *korifiv1alpha1.CFDomain) string {
	pathDetails := ""

	if route.Spec.Path != "" {
		pathDetails = fmt.Sprintf(" and path '%s'", route.Spec.Path)
	}

	return fmt.Sprintf("Route already exists with host '%s'%s for domain '%s'.",
		route.Spec.Host, pathDetails, domain.Spec.Name)
}

func uniqueName(route korifiv1alpha1.CFRoute) string {
	return strings.Join([]string{strings.ToLower(route.Spec.Host), route.Spec.DomainRef.Namespace, route.Spec.DomainRef.Name, route.Spec.Path}, "::")
}

func validateFQDN(host, domain string) error {
	// we only need to validate that "<host>.<domain>" is not too long and that
	// <host> is either "*" or a valid dns label. The domain webhook already
	// guarantees that the domain is well formed
	if len(host+"."+domain) > validation.DNS1123SubdomainMaxLength {
		return webhooks.ValidationError{
			Type:    RouteSubdomainValidationErrorType,
			Message: fmt.Sprintf("A valid DNS-1123 subdomain must not exceed %d characters.", validation.DNS1123SubdomainMaxLength),
		}.ExportJSONError()
	}

	err := validateHost(host)
	if err != nil {
		return webhooks.ValidationError{
			Type:    RouteHostNameValidationErrorType,
			Message: fmt.Sprintf("Host %q is not valid: %s", host, err.Error()),
		}.ExportJSONError()
	}

	return nil
}

func validateHost(host string) error {
	if host == "*" {
		return nil
	}

	var multiErr *multierror.Error
	for _, err := range validation.IsDNS1123Label(host) {
		multiErr = multierror.Append(multiErr, errors.New(err))
	}

	if multiErr == nil {
		return nil
	}

	return multiErr.ErrorOrNil()
}

func validatePath(path string) error {
	var errStrings []string

	if path == "" {
		return nil
	}

	_, err := url.ParseRequestURI(path)
	if err != nil {
		errStrings = append(errStrings, InvalidURIError)
	}

	if path == "/" {
		errStrings = append(errStrings, PathIsSlashError)
	}

	if strings.Contains(path, "?") {
		errStrings = append(errStrings, PathHasQuestionMarkError)
	}

	if len(path) > 128 {
		errStrings = append(errStrings, PathLengthExceededError)
	}

	if len(errStrings) == 0 {
		return nil
	}

	if len(errStrings) > 0 {
		return webhooks.ValidationError{
			Type:    RoutePathValidationErrorType,
			Message: strings.Join(errStrings, ", "),
		}.ExportJSONError()
	}

	return nil
}

func (v *CFRouteValidator) checkDestinationsExistInNamespace(ctx context.Context, route korifiv1alpha1.CFRoute) error {
	for _, destination := range route.Spec.Destinations {
		err := v.client.Get(ctx, client.ObjectKey{Namespace: route.Namespace, Name: destination.AppRef.Name}, &korifiv1alpha1.CFApp{})
		if err != nil {
			return err
		}
	}

	return nil
}
