package relationships

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-controllers-space-guid,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfapps;cfbuilds;cfpackages;cfprocesses;cfservicebindings;cfserviceinstances;cftasks,verbs=create;update,versions=v1alpha1,name=mcfspaceguid.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"code.cloudfoundry.org/korifi/tools"
	"github.com/go-logr/logr"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var spaceguidlog = log.Log.WithName("spaceguid-webhook")

type SpaceGUIDWebhook struct {
	decoder admission.Decoder
}

func NewSpaceGUIDWebhook() *SpaceGUIDWebhook {
	return &SpaceGUIDWebhook{}
}

func (r *SpaceGUIDWebhook) SetupWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register("/mutate-korifi-cloudfoundry-org-v1alpha1-controllers-space-guid", &admission.Webhook{
		Handler: r,
	})
	r.decoder = admission.NewDecoder(mgr.GetScheme())
}

func (r *SpaceGUIDWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	var obj metav1.PartialObjectMetadata

	if err := r.decoder.Decode(req, &obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	logger := spaceguidlog.WithValues("kind", obj.GetObjectKind(), "namespace", obj.GetNamespace(), "name", obj.GetName())

	switch req.Operation {
	case admissionv1.Create:
		logger.V(1).Info("adding-space-guid-on-create")

		return r.setSpaceGUID(obj)
	case admissionv1.Update:
		return r.ensureSpaceGUIDImmutable(logger, obj, req)
	default:
		logger.Info("received-unexpected-operation-type", "operation", req.Operation)
		return admission.Denied("we only accept create/update")
	}
}

func (r *SpaceGUIDWebhook) setSpaceGUID(obj metav1.PartialObjectMetadata) admission.Response {
	origMarshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	obj.SetLabels(
		tools.SetMapValue(obj.GetLabels(), korifiv1alpha1.SpaceGUIDKey, obj.GetNamespace()),
	)

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(origMarshalled, marshalled)
}

func (r *SpaceGUIDWebhook) ensureSpaceGUIDImmutable(logger logr.Logger, obj metav1.PartialObjectMetadata, req admission.Request) admission.Response {
	var oldObj metav1.PartialObjectMetadata
	if err := r.decoder.DecodeRaw(req.OldObject, &oldObj); err != nil {
		logger.Error(err, "failed-to-decode-old-object")
		return admission.Errored(http.StatusBadRequest, err)
	}

	if oldObj.Labels[korifiv1alpha1.SpaceGUIDKey] != obj.Labels[korifiv1alpha1.SpaceGUIDKey] {
		return admission.Denied(fmt.Sprintf("Label %q is immutable", korifiv1alpha1.SpaceGUIDKey))
	}

	return admission.Allowed("")
}
