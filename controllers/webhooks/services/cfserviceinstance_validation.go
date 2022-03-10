package services

import (
	"context"
	"errors"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"
	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name NameValidator . NameValidator

type NameValidator interface {
	ValidateCreate(ctx context.Context, logger logr.Logger, namespace, newName string) error
	ValidateUpdate(ctx context.Context, logger logr.Logger, namespace, oldName, newName string) error
	ValidateDelete(ctx context.Context, logger logr.Logger, namespace, oldName string) error
}

const (
	ServiceInstanceEntityType = "serviceinstance"
)

var cfserviceinstancelog = logf.Log.WithName("cfserviceinstance-validate")

//+kubebuilder:webhook:path=/validate-services-cloudfoundry-org-v1alpha1-cfserviceinstance,mutating=false,failurePolicy=fail,sideEffects=None,groups=services.cloudfoundry.org,resources=cfserviceinstances,verbs=create;update;delete,versions=v1alpha1,name=vcfserviceinstance.services.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFServiceInstanceValidation struct {
	decoder            *admission.Decoder
	duplicateValidator NameValidator
}

func NewCFServiceInstanceValidation(duplicateValidator NameValidator) *CFServiceInstanceValidation {
	return &CFServiceInstanceValidation{
		duplicateValidator: duplicateValidator,
	}
}

func (v *CFServiceInstanceValidation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-services-cloudfoundry-org-v1alpha1-cfserviceinstance", &webhook.Admission{Handler: v})

	return nil
}

func (v *CFServiceInstanceValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	cfserviceinstancelog.Info("Validate", "name", req.Name)

	var cfServiceInstance, oldCFServiceInstance v1alpha1.CFServiceInstance
	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		err := v.decoder.Decode(req, &cfServiceInstance)
		if err != nil {
			errMessage := "Error while decoding CFServiceInstance object"
			cfserviceinstancelog.Error(err, errMessage)

			return admission.Denied(errMessage)
		}
	}
	if req.Operation == admissionv1.Update || req.Operation == admissionv1.Delete {
		err := v.decoder.DecodeRaw(req.OldObject, &oldCFServiceInstance)
		if err != nil {
			errMessage := "Error while decoding old CFServiceInstance object"
			cfserviceinstancelog.Error(err, errMessage)

			return admission.Denied(errMessage)
		}
	}

	var validatorErr error
	switch req.Operation {
	case admissionv1.Create:
		validatorErr = v.duplicateValidator.ValidateCreate(ctx, cfserviceinstancelog, cfServiceInstance.Namespace, cfServiceInstance.Spec.Name)

	case admissionv1.Update:
		validatorErr = v.duplicateValidator.ValidateUpdate(ctx, cfserviceinstancelog, cfServiceInstance.Namespace, oldCFServiceInstance.Spec.Name, cfServiceInstance.Spec.Name)

	case admissionv1.Delete:
		validatorErr = v.duplicateValidator.ValidateDelete(ctx, cfserviceinstancelog, oldCFServiceInstance.Namespace, oldCFServiceInstance.Spec.Name)
	}

	if validatorErr != nil {
		if errors.Is(validatorErr, webhooks.ErrorDuplicateName) {
			return admission.Denied(webhooks.DuplicateServiceInstanceNameError.Marshal())
		}

		return admission.Denied(webhooks.UnknownError.Marshal())
	}

	return admission.Allowed("")
}

func (v *CFServiceInstanceValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
