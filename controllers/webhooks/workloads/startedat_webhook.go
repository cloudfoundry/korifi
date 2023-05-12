package workloads

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-cfapp-startedat,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfapps,verbs=update,versions=v1alpha1,name=mcfappstartedat.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// AppStartedAtWebhook does not implement the admission.Defaulter interface as we
// need to access both oldObject and (new)Object to determine the actual state
// change. So we use the lower-level admission.Handler interface
type AppStartedAtWebhook struct {
	decoder *admission.Decoder
}

var startedatlog = logf.Log.WithName("app-startedat-webhook")

func (r *AppStartedAtWebhook) SetupWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register("/mutate-korifi-cloudfoundry-org-v1alpha1-cfapp-startedat", &admission.Webhook{
		Handler: r,
	})
}

func (r *AppStartedAtWebhook) InjectDecoder(d *admission.Decoder) error {
	r.decoder = d
	return nil
}

func (r *AppStartedAtWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	startedatlog.Info("StartedAt Webhook Handle()")
	var cfApp korifiv1alpha1.CFApp
	if err := r.decoder.Decode(req, &cfApp); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	var oldCFApp korifiv1alpha1.CFApp
	if err := r.decoder.DecodeRaw(req.OldObject, &oldCFApp); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if cfApp.Spec.DesiredState == korifiv1alpha1.StartedState && oldCFApp.Spec.DesiredState == korifiv1alpha1.StoppedState {
		if cfApp.Annotations == nil {
			cfApp.Annotations = map[string]string{}
		}
		cfApp.Annotations[korifiv1alpha1.StartedAtAnnotation] = time.Now().Format(time.RFC3339)
	}

	marshalled, err := json.Marshal(cfApp)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshalled)
}
