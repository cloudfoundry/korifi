package networking

import (
	"context"
	"errors"
	"regexp"
	"strings"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	RouteEntityType = "route"
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
	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		err := v.decoder.Decode(req, &route)
		if err != nil {
			errMessage := "Error while decoding CFRoute object"
			logger.Error(err, errMessage)

			return admission.Denied(errMessage)
		}
	}
	if req.Operation == admissionv1.Update || req.Operation == admissionv1.Delete {
		err := v.decoder.DecodeRaw(req.OldObject, &oldRoute)
		if err != nil {
			errMessage := "Error while decoding old CFRoute object"
			logger.Error(err, errMessage)

			return admission.Denied(errMessage)
		}
	}

	var validatorErr error
	switch req.Operation {
	case admissionv1.Create:
		_, err := isHost(route.Spec.Host)
		if err != nil {
			return admission.Denied(err.Error())
		}
		_, err = v.isFQDN(route)
		if err != nil {
			return admission.Denied(err.Error())
		}
		validatorErr = v.duplicateValidator.ValidateCreate(ctx, logger, v.rootNamespace, uniqueName(route))

	case admissionv1.Update:
		_, err := isHost(route.Spec.Host)
		if err != nil {
			return admission.Denied(err.Error())
		}
		_, err = v.isFQDN(route)
		if err != nil {
			return admission.Denied(err.Error())
		}
		validatorErr = v.duplicateValidator.ValidateUpdate(ctx, logger, v.rootNamespace, uniqueName(oldRoute), uniqueName(route))

	case admissionv1.Delete:
		validatorErr = v.duplicateValidator.ValidateDelete(ctx, logger, v.rootNamespace, uniqueName(oldRoute))
	}

	if validatorErr != nil {
		logger.Info("duplicate validation failed", "error", validatorErr)

		if errors.Is(validatorErr, webhooks.ErrorDuplicateName) {
			return admission.Denied(webhooks.DuplicateRouteError.Marshal())
		}

		return admission.Denied(webhooks.UnknownError.Marshal())
	}

	return admission.Allowed("")
}

func uniqueName(route networkingv1alpha1.CFRoute) string {
	return strings.Join([]string{route.Spec.Host, route.Spec.DomainRef.Namespace, route.Spec.DomainRef.Name, route.Spec.Path}, "::")
}

func (v *CFRouteValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}

func (v *CFRouteValidation) isFQDN(cfRoute networkingv1alpha1.CFRoute) (bool, error) {
	var cfDomain networkingv1alpha1.CFDomain
	ctx := context.Background()
	err := v.Client.Get(ctx, types.NamespacedName{Name: cfRoute.Spec.DomainRef.Name, Namespace: cfRoute.Spec.DomainRef.Namespace}, &cfDomain)
	if err != nil {
		return false, err
	}

	return IsFQDN(cfRoute.Spec.Host, cfDomain.Spec.Name)
}

func isHost(hostname string) (bool, error) {
	const (
		// HOST_REGEX - Must be either "*" or contain only alphanumeric characters, "_", or "-"
		HOST_REGEX                  = "^([\\w\\-]+|\\*)?$"
		MAXIMUM_DOMAIN_LABEL_LENGTH = 63
	)

	rxHost := regexp.MustCompile(HOST_REGEX)

	if len(hostname) == 0 {
		return false, errors.New("host cannot be empty")
	}

	if len(hostname) > MAXIMUM_DOMAIN_LABEL_LENGTH {
		return false, errors.New("host is too long (maximum is 63 characters)")
	}

	if !rxHost.MatchString(hostname) {
		return false, errors.New("host must be either \"*\" or contain only alphanumeric characters, \"_\", or \"-\"")
	}

	return true, nil
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
		return false, errors.New("FQDN does not comply with RFC 1035 standards")
	}

	return true, nil
}
