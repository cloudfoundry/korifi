package guid

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-controllers-guid,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfapps;cfspaces;cfpackages;cforgs;cfroutes;cfservicebindings;cfserviceinstances;cfbuild,verbs=create,versions=v1alpha1,name=mcfguid.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var guidlog = logf.Log.WithName("guid-webhook")

type ControllersGUIDWebhook struct {
	decoder admission.Decoder
}

func NewControllersGUIDWebhook() *ControllersGUIDWebhook {
	return &ControllersGUIDWebhook{}
}

func (r *ControllersGUIDWebhook) SetupWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register("/mutate-korifi-cloudfoundry-org-v1alpha1-controllers-guid", &admission.Webhook{
		Handler: r,
	})
	r.decoder = admission.NewDecoder(mgr.GetScheme())
}

func (r *ControllersGUIDWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	var obj metav1.PartialObjectMetadata

	if err := r.decoder.Decode(req, &obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	logger := guidlog.WithValues("kind", obj.GetObjectKind(), "namespace", obj.GetNamespace(), "name", obj.GetName())

	origMarshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	obj.SetName(uuid.NewString())
	logger.Info("guid set", "guid", obj.GetName())

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(origMarshalled, marshalled)
}
