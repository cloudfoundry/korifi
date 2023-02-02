package workloads

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-cfapp-apprev,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfapps,verbs=update,versions=v1alpha1,name=mcfapprev.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// AppRevWebhook does not implement the admission.Defaulter interface as we
// need to access both oldObject and (new)Object to determine the actual state
// change. So we use the lower-level admission.Handler interface
type AppRevWebhook struct {
	decoder *admission.Decoder
}

var apprevlog = logf.Log.WithName("apprev-webhook")

func (r *AppRevWebhook) SetupWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register("/mutate-korifi-cloudfoundry-org-v1alpha1-cfapp-apprev", &admission.Webhook{
		Handler: r,
	})
}

func (r *AppRevWebhook) InjectDecoder(d *admission.Decoder) error {
	r.decoder = d
	return nil
}

func (r *AppRevWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	var cfApp korifiv1alpha1.CFApp
	if err := r.decoder.Decode(req, &cfApp); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	var oldCFApp korifiv1alpha1.CFApp
	if err := r.decoder.DecodeRaw(req.OldObject, &oldCFApp); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if cfApp.Spec.DesiredState == korifiv1alpha1.StoppedState && oldCFApp.Spec.DesiredState == korifiv1alpha1.StartedState {
		cfApp.Annotations[korifiv1alpha1.CFAppRevisionKey] = bumpAppRev(cfApp.Annotations[korifiv1alpha1.CFAppRevisionKey])
	}

	marshalled, err := json.Marshal(cfApp)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshalled)
}

func bumpAppRev(currentRevValue string) string {
	revValue, err := strconv.Atoi(currentRevValue)
	if err != nil || revValue < 0 {
		apprevlog.Info("setting-invalid-app-rev-to-zero", "app-rev", currentRevValue)
		return korifiv1alpha1.CFAppRevisionKeyDefault
	}

	return strconv.Itoa(revValue + 1)
}
