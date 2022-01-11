package networking

import (
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads"
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	admissionv1 "k8s.io/api/admission/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	RouteEntityType = "route"
)

var cfroutelog = logf.Log.WithName("cfroute-validate")

//+kubebuilder:webhook:path=/validate-networking-cloudfoundry-org-v1alpha1-cfroute,mutating=false,failurePolicy=fail,sideEffects=None,groups=networking.cloudfoundry.org,resources=cfroutes,verbs=create;update;delete,versions=v1alpha1,name=vcfroute.networking.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFRouteValidation struct {
	decoder           *admission.Decoder
	routeNameRegistry workloads.NameRegistry
}

func NewCFRouteValidation(routeNameRegistry workloads.NameRegistry) *CFRouteValidation {
	return &CFRouteValidation{
		routeNameRegistry: routeNameRegistry,
	}
}

func (v *CFRouteValidation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-networking-cloudfoundry-org-v1alpha1-cfroute", &webhook.Admission{Handler: v})

	return nil
}

func (v *CFRouteValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	cfroutelog.Info("Validate", "name", req.Name)

	var cfRoute, oldCFRoute v1alpha1.CFRoute
	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		err := v.decoder.Decode(req, &cfRoute)
		if err != nil {
			errMessage := "Error while decoding CFRoute object"
			cfroutelog.Error(err, errMessage)

			return admission.Denied(errMessage)
		}
	}
	if req.Operation == admissionv1.Update || req.Operation == admissionv1.Delete {
		err := v.decoder.DecodeRaw(req.OldObject, &oldCFRoute)
		if err != nil {
			errMessage := "Error while decoding old CFRoute object"
			cfroutelog.Error(err, errMessage)

			return admission.Denied(errMessage)
		}
	}

	switch req.Operation {
	case admissionv1.Create:
		err := v.routeNameRegistry.RegisterName(ctx, cfRoute.Namespace, cfRoute.Spec.Host+"_"+cfRoute.Spec.DomainRef.Name)
		if k8serrors.IsAlreadyExists(err) {
			cfroutelog.Info("route name already exists",
				"name", cfRoute.Name,
				"namespace", cfRoute.Namespace,
				"host", cfRoute.Spec.Host,
			)

			return admission.Denied(DuplicateRouteError.Marshal())
		}
		if err != nil {
			cfroutelog.Error(err, "failed to register name")

			return admission.Denied(UnknownError.Marshal())
		}

	case admissionv1.Update:
		oldName := oldCFRoute.Spec.Host + "_" + oldCFRoute.Spec.DomainRef.Name
		newName := cfRoute.Spec.Host + "_" + cfRoute.Spec.DomainRef.Name
		if oldName == newName {
			return admission.Allowed("")
		}

		err := v.routeNameRegistry.TryLockName(ctx, oldCFRoute.Namespace, oldName)
		if err != nil {
			cfroutelog.Info("failed to acquire lock on old route during update",
				"error", err,
				"host", oldCFRoute.Spec.Host,
				"namespace", oldCFRoute.Namespace,
			)
			return admission.Denied(UnknownError.Marshal())
		}

		err = v.routeNameRegistry.RegisterName(ctx, cfRoute.Namespace, newName)
		if err != nil {
			unlockErr := v.routeNameRegistry.UnlockName(ctx, oldCFRoute.Namespace, oldName)
			if unlockErr != nil {
				// A locked registry entry will remain, so future name updates will fail until operator intervenes
				cfroutelog.Error(unlockErr, "failed to release registry lock on old route",
					"namespace", oldCFRoute.Namespace,
					"host", oldName,
				)
			}

			if k8serrors.IsAlreadyExists(err) {
				cfroutelog.Info("route already exists",
					"namespace", cfRoute.Namespace,
					"host", cfRoute.Spec.Host,
				)
				return admission.Denied(DuplicateRouteError.Marshal())
			}

			cfroutelog.Info("failed to acquire lock on old name during update",
				"error", err,
				"host", oldCFRoute.Spec.Host,
				"namespace", oldCFRoute.Namespace,
			)
			return admission.Denied(UnknownError.Marshal())
		}

		err = v.routeNameRegistry.DeregisterName(ctx, oldCFRoute.Namespace, oldName)
		if err != nil {
			// We cannot unclaim the old name. It will remain claimed until an operator intervenes.
			cfroutelog.Error(err, "failed to deregister old name during update",
				"namespace", oldCFRoute.Namespace,
				"host", oldCFRoute.Spec.Host,
			)
		}

	case admissionv1.Delete:
		oldName := oldCFRoute.Spec.Host + "_" + oldCFRoute.Spec.DomainRef.Name
		err := v.routeNameRegistry.DeregisterName(ctx, oldCFRoute.Namespace, oldName)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				cfroutelog.Info("cannot deregister name: registry entry for name not found",
					"namespace", oldCFRoute.Namespace,
					"host", oldCFRoute.Spec.Host,
				)

				return admission.Allowed("")
			}

			cfroutelog.Error(err, "failed to deregister name during delete",
				"namespace", oldCFRoute.Namespace,
				"host", oldCFRoute.Spec.Host,
			)

			return admission.Denied(UnknownError.Marshal())
		}
	}

	return admission.Allowed("")
}

func (v *CFRouteValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
