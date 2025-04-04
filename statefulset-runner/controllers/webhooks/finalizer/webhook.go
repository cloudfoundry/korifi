package finalizer

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-appworkload,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=appworkloads,verbs=create,versions=v1alpha1,name=mappworkload.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

import (
	"context"
	"encoding/json"
	"net/http"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const AppWorkloadFinalizerName = "appWorkload.korifi.cloudfoundry.org"

type Webhook struct {
	decoder admission.Decoder
}

var log = logf.Log.WithName("appworkload-webhook")

func NewWebhook() *Webhook {
	return &Webhook{}
}

func (r *Webhook) SetupWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register("/mutate-korifi-cloudfoundry-org-v1alpha1-appworkload", &admission.Webhook{
		Handler: r,
	})
	r.decoder = admission.NewDecoder(mgr.GetScheme())
}

func (r *Webhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	var workload korifiv1alpha1.AppWorkload
	if err := r.decoder.Decode(req, &workload); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if controllerutil.AddFinalizer(&workload, AppWorkloadFinalizerName) {
		log.Info("adding-finalizer to appWorkload", "name", workload.Name)
	}

	rawUpdatedWorkload, err := json.Marshal(workload)
	if err != nil {
		log.Error(err, "failed to marshall appworkload")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, rawUpdatedWorkload)
}
