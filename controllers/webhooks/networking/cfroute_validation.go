package networking

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	RouteEntityType = "route"

	RouteDecodingError                     = "RouteDecodingError"
	DuplicateRouteError                    = "DuplicateRouteError"
	RouteDestinationNotInSpaceError        = "RouteDestinationNotInSpaceError"
	RouteDestinationNotInSpaceErrorMessage = "Route destination app not found in space"
	RouteHostNameValidationError           = "RouteHostNameValidationError"
	RoutePathValidationError               = "RoutePathValidationError"
	RouteFQDNValidationError               = "RouteFQDNValidationError"
	RouteFQDNValidationErrorMessage        = "FQDN does not comply with RFC 1035 standards"

	HostEmptyError  = "host cannot be empty"
	HostLengthError = "host is too long (maximum is 63 characters)"
	HostFormatError = "host must be either \"*\" or contain only alphanumeric characters, \"_\", or \"-\""

	InvalidURIError          = "Invalid Route URI"
	PathIsSlashError         = "Path cannot be a single slash"
	PathHasQuestionMarkError = "Path cannot contain a question mark"
	PathLengthExceededError  = "Path cannot exceed 128 characters"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name NameValidator . NameValidator

type NameValidator interface {
	ValidateCreate(ctx context.Context, logger logr.Logger, namespace, newName string) error
	ValidateUpdate(ctx context.Context, logger logr.Logger, namespace, oldName, newName string) error
	ValidateDelete(ctx context.Context, logger logr.Logger, namespace, oldName string) error
}

var logger = logf.Log.WithName("route-validation")

//+kubebuilder:webhook:path=/validate-networking-cloudfoundry-org-v1alpha1-cfroute,mutating=false,failurePolicy=fail,sideEffects=None,groups=networking.cloudfoundry.org,resources=cfroutes,verbs=create;update;delete,versions=v1alpha1,name=vcfroute.networking.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFRouteValidation struct {
	decoder            *admission.Decoder
	rootNamespace      string
	duplicateValidator NameValidator
	Client             client.Client
}

func NewCFRouteValidation(nameValidator NameValidator, rootNamespace string, client client.Client) *CFRouteValidation {
	return &CFRouteValidation{
		duplicateValidator: nameValidator,
		rootNamespace:      rootNamespace,
		Client:             client,
	}
}

func (v *CFRouteValidation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-networking-cloudfoundry-org-v1alpha1-cfroute", &webhook.Admission{Handler: v})

	return nil
}

func (v *CFRouteValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	var route, oldRoute networkingv1alpha1.CFRoute
	var domain networkingv1alpha1.CFDomain
	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		err := v.decoder.Decode(req, &route)
		if err != nil { // untested
			errMessage := "Error while decoding CFRoute object"
			logger.Error(err, errMessage)
			return admission.Denied(webhooks.ValidationError{Type: RouteDecodingError, Message: errMessage}.Marshal())
		}

		err = v.Client.Get(ctx, types.NamespacedName{Name: route.Spec.DomainRef.Name, Namespace: route.Spec.DomainRef.Namespace}, &domain)
		if err != nil {
			errMessage := "Error while retrieving CFDomain object"
			logger.Error(err, errMessage)
			validationError := webhooks.ValidationError{
				Type:    webhooks.UnknownError,
				Message: errMessage,
			}
			return admission.Denied(validationError.Marshal())
		}
	}
	if req.Operation == admissionv1.Update || req.Operation == admissionv1.Delete {
		err := v.decoder.DecodeRaw(req.OldObject, &oldRoute)
		if err != nil { // untested
			errMessage := "Error while decoding old CFRoute object"
			logger.Error(err, errMessage)
			return admission.Denied(webhooks.ValidationError{Type: RouteDecodingError, Message: errMessage}.Marshal())
		}
	}

	var validatorErr error
	switch req.Operation {
	case admissionv1.Create:
		if err := isHost(route.Spec.Host); err != nil {
			return admission.Denied(err.Error())
		}
		if _, err := IsFQDN(route.Spec.Host, domain.Spec.Name); err != nil {
			return admission.Denied(err.Error())
		}
		if err := validatePath(route); err != nil {
			return admission.Denied(err.Error())
		}

		if err := v.checkDestinationsExistInNamespace(ctx, route); err != nil {
			if k8serrors.IsNotFound(err) {
				return admission.Denied(webhooks.ValidationError{Type: RouteDestinationNotInSpaceError, Message: RouteDestinationNotInSpaceErrorMessage}.Marshal())
			}
			errMessage := "Error while checking Route Destinations in Namespace"
			logger.Error(err, errMessage)
			validationError := webhooks.ValidationError{
				Type:    webhooks.UnknownError,
				Message: errMessage,
			}
			return admission.Denied(validationError.Marshal())
		}

		validatorErr = v.duplicateValidator.ValidateCreate(ctx, logger, v.rootNamespace, uniqueName(route))

	case admissionv1.Update:
		if err := isHost(route.Spec.Host); err != nil {
			return admission.Denied(err.Error())
		}
		if _, err := IsFQDN(route.Spec.Host, domain.Spec.Name); err != nil {
			return admission.Denied(err.Error())
		}
		if err := validatePath(route); err != nil {
			return admission.Denied(err.Error())
		}

		if err := v.checkDestinationsExistInNamespace(ctx, route); err != nil {
			if k8serrors.IsNotFound(err) {
				return admission.Denied(webhooks.ValidationError{Type: RouteDestinationNotInSpaceError, Message: RouteDestinationNotInSpaceErrorMessage}.Marshal())
			}
			errMessage := "Error while checking Route Destinations in Namespace"
			logger.Error(err, errMessage)
			validationError := webhooks.ValidationError{
				Type:    webhooks.UnknownError,
				Message: errMessage,
			}
			return admission.Denied(validationError.Marshal())
		}

		validatorErr = v.duplicateValidator.ValidateUpdate(ctx, logger, v.rootNamespace, uniqueName(oldRoute), uniqueName(route))

	case admissionv1.Delete:
		validatorErr = v.duplicateValidator.ValidateDelete(ctx, logger, v.rootNamespace, uniqueName(oldRoute))
	}

	if validatorErr != nil {
		logger.Info("duplicate validation failed", "error", validatorErr)

		if errors.Is(validatorErr, webhooks.ErrorDuplicateName) {
			pathDetails := ""
			if route.Spec.Path != "" {
				pathDetails = fmt.Sprintf(" and path '%s'", route.Spec.Path)
			}
			errorDetail := fmt.Sprintf("Route already exists with host '%s'%s for domain '%s'.",
				route.Spec.Host, pathDetails, domain.Spec.Name)

			ve := webhooks.ValidationError{
				Type:    DuplicateRouteError,
				Message: errorDetail,
			}
			return admission.Denied(ve.Marshal())
		}

		errMessage := "Unknown error while checking Route Name Duplicate"
		logger.Error(validatorErr, errMessage)
		validationError := webhooks.ValidationError{
			Type:    webhooks.UnknownError,
			Message: errMessage,
		}
		return admission.Denied(validationError.Marshal())
	}

	return admission.Allowed("")
}

func uniqueName(route networkingv1alpha1.CFRoute) string {
	return strings.Join([]string{route.Spec.Host, route.Spec.DomainRef.Namespace, route.Spec.DomainRef.Name, route.Spec.Path}, "::")
}

func validatePath(route networkingv1alpha1.CFRoute) error {
	var errStrings []string

	if route.Spec.Path == "" {
		return nil
	}

	_, err := url.ParseRequestURI(route.Spec.Path)
	if err != nil {
		errStrings = append(errStrings, InvalidURIError)
	}
	if route.Spec.Path == "/" {
		errStrings = append(errStrings, PathIsSlashError)
	}
	if strings.Contains(route.Spec.Path, "?") {
		errStrings = append(errStrings, PathHasQuestionMarkError)
	}
	if len(route.Spec.Path) > 128 {
		errStrings = append(errStrings, PathLengthExceededError)
	}
	if len(errStrings) == 0 {
		return nil
	}

	if len(errStrings) > 0 {
		ve := webhooks.ValidationError{
			Type:    RoutePathValidationError,
			Message: strings.Join(errStrings, ", "),
		}
		return errors.New(ve.Marshal())
	}

	return nil
}

func (v *CFRouteValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}

func isHost(hostname string) error {
	const (
		// HOST_REGEX - Must be either "*" or contain only alphanumeric characters, "_", or "-"
		HOST_REGEX                  = "^([\\w\\-]+|\\*)?$"
		MAXIMUM_DOMAIN_LABEL_LENGTH = 63
	)

	var errStrings []string

	rxHost := regexp.MustCompile(HOST_REGEX)

	if len(hostname) == 0 {
		errStrings = append(errStrings, HostEmptyError)
	}

	if len(hostname) > MAXIMUM_DOMAIN_LABEL_LENGTH {
		errStrings = append(errStrings, HostLengthError)
	}

	if !rxHost.MatchString(hostname) {
		errStrings = append(errStrings, HostFormatError)
	}

	if len(errStrings) > 0 {
		ve := webhooks.ValidationError{
			Type:    RouteHostNameValidationError,
			Message: strings.Join(errStrings, ", "),
		}
		return errors.New(ve.Marshal())
	}

	return nil
}

func IsFQDN(host, domain string) (bool, error) {
	const (
		// MAXIMUM_FQDN_DOMAIN_LENGTH - The maximum fully-qualified domain length is 255 including separators, but this includes two "invisible"
		// characters at the beginning and end of the domain, so for string comparisons, the correct length is 253.
		//
		// The first character denotes the length of the first label, and the last character denotes the termination
		// of the domain.
		MAXIMUM_FQDN_DOMAIN_LENGTH = 253
		MINIMUM_FQDN_DOMAIN_LENGTH = 3
		DOMAIN_REGEX               = "^(([a-z0-9]|[a-z0-9][a-z0-9\\-]{0,61}[a-z0-9])\\.)+([a-z0-9]|[a-z0-9][a-z0-9\\-]{0,61}[a-z0-9])$"
		SUBDOMAIN_REGEX            = "^([^\\.]{0,63}\\.)*[^\\.]{0,63}$"
	)

	fqdn := host + "." + domain

	rxSubdomain := regexp.MustCompile(SUBDOMAIN_REGEX)

	if !rxSubdomain.MatchString(fqdn) {
		return false, errors.New("subdomains must each be at most 63 characters")
	}

	rxDomain := regexp.MustCompile(DOMAIN_REGEX)
	fqdnLength := len(fqdn)

	if fqdnLength < MINIMUM_FQDN_DOMAIN_LENGTH || fqdnLength > MAXIMUM_FQDN_DOMAIN_LENGTH || !rxDomain.MatchString(fqdn) {
		return false, errors.New(webhooks.ValidationError{Type: RouteFQDNValidationError, Message: RouteFQDNValidationErrorMessage}.Marshal())
	}

	return true, nil
}

func (v *CFRouteValidation) checkDestinationsExistInNamespace(ctx context.Context, route networkingv1alpha1.CFRoute) error {
	for _, destination := range route.Spec.Destinations {
		if err := v.Client.Get(
			ctx, client.ObjectKey{Namespace: route.Namespace, Name: destination.AppRef.Name}, &workloadsv1alpha1.CFApp{},
		); err != nil {
			return err
		}
	}

	return nil
}
