package workloads

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	admissionv1 "k8s.io/api/admission/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	AppEntityType = "app"
)

var cfapplog = logf.Log.WithName("cfapp-validate")

//+kubebuilder:webhook:path=/validate-workloads-cloudfoundry-org-v1alpha1-cfapp,mutating=false,failurePolicy=fail,sideEffects=None,groups=workloads.cloudfoundry.org,resources=cfapps,verbs=create;update;delete,versions=v1alpha1,name=vcfapp.workloads.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFAppValidation struct {
	decoder         *admission.Decoder
	appNameRegistry NameRegistry
}

func NewCFAppValidation(appNameRegistry NameRegistry) *CFAppValidation {
	return &CFAppValidation{
		appNameRegistry: appNameRegistry,
	}
}

func (v *CFAppValidation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-workloads-cloudfoundry-org-v1alpha1-cfapp", &webhook.Admission{Handler: v})

	return nil
}

func (v *CFAppValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	cfapplog.Info("Validate", "name", req.Name)

	var cfApp, oldCFApp v1alpha1.CFApp
	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		err := v.decoder.Decode(req, &cfApp)
		if err != nil {
			errMessage := "Error while decoding CFApp object"
			cfapplog.Error(err, errMessage)

			return admission.Denied(errMessage)
		}
	}
	if req.Operation == admissionv1.Update || req.Operation == admissionv1.Delete {
		err := v.decoder.DecodeRaw(req.OldObject, &oldCFApp)
		if err != nil {
			errMessage := "Error while decoding old CFApp object"
			cfapplog.Error(err, errMessage)

			return admission.Denied(errMessage)
		}
	}

	switch req.Operation {
	case admissionv1.Create:
		err := v.appNameRegistry.RegisterName(ctx, cfApp.Namespace, cfApp.Spec.Name)
		if k8serrors.IsAlreadyExists(err) {
			cfapplog.Info("app name already exists",
				"name", cfApp.Name,
				"namespace", cfApp.Namespace,
				"app-name", cfApp.Spec.Name,
			)

			return admission.Denied(DuplicateAppError.Marshal())
		}
		if err != nil {
			cfapplog.Error(err, "failed to register name")

			return admission.Denied(UnknownError.Marshal())
		}

	case admissionv1.Update:
		if oldCFApp.Spec.Name == cfApp.Spec.Name {
			return admission.Allowed("")
		}

		err := v.appNameRegistry.TryLockName(ctx, oldCFApp.Namespace, oldCFApp.Spec.Name)
		if err != nil {
			cfapplog.Info("failed to acquire lock on old name during update",
				"error", err,
				"name", oldCFApp.Spec.Name,
				"namespace", oldCFApp.Namespace,
			)
			return admission.Denied(UnknownError.Marshal())
		}

		err = v.appNameRegistry.RegisterName(ctx, cfApp.Namespace, cfApp.Spec.Name)
		if err != nil {
			unlockErr := v.appNameRegistry.UnlockName(ctx, oldCFApp.Namespace, oldCFApp.Spec.Name)
			if unlockErr != nil {
				// A locked registry entry will remain, so future name updates will fail until operator intervenes
				cfapplog.Error(unlockErr, "failed to release registry lock on old name",
					"namespace", oldCFApp.Namespace,
					"name", oldCFApp.Spec.Name,
				)
			}

			if k8serrors.IsAlreadyExists(err) {
				cfapplog.Info("app name already exists",
					"name", cfApp.Name,
					"namespace", cfApp.Namespace,
					"app-name", cfApp.Spec.Name,
				)
				return admission.Denied(DuplicateAppError.Marshal())
			}

			cfapplog.Info("failed to acquire lock on old name during update",
				"error", err,
				"name", oldCFApp.Spec.Name,
				"namespace", oldCFApp.Namespace,
			)
			return admission.Denied(UnknownError.Marshal())
		}

		err = v.appNameRegistry.DeregisterName(ctx, oldCFApp.Namespace, oldCFApp.Spec.Name)
		if err != nil {
			// We cannot unclaim the old name. It will remain claimed until an operator intervenes.
			cfapplog.Error(err, "failed to deregister old name during update",
				"namespace", oldCFApp.Namespace,
				"name", oldCFApp.Spec.Name,
			)
		}

	case admissionv1.Delete:
		err := v.appNameRegistry.DeregisterName(ctx, oldCFApp.Namespace, oldCFApp.Spec.Name)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				cfapplog.Info("cannot deregister name: registry entry for name not found",
					"namespace", oldCFApp.Namespace,
					"name", oldCFApp.Spec.Name,
				)

				return admission.Allowed("")
			}

			cfapplog.Error(err, "failed to deregister name during delete",
				"namespace", oldCFApp.Namespace,
				"name", oldCFApp.Spec.Name,
			)

			return admission.Denied(UnknownError.Marshal())
		}
	}

	return admission.Allowed("")
}

func (v *CFAppValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
