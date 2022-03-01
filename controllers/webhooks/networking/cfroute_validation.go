package networking

import (
	"context"
	"errors"
	"strings"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"
	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	ctrl "sigs.k8s.io/controller-runtime"
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
}

func NewCFRouteValidation(nameValidator NameValidator, rootNamespace string) *CFRouteValidation {
	return &CFRouteValidation{
		duplicateValidator: nameValidator,
		rootNamespace:      rootNamespace,
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
		validatorErr = v.duplicateValidator.ValidateCreate(ctx, logger, v.rootNamespace, uniqueName(route))

	case admissionv1.Update:
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
