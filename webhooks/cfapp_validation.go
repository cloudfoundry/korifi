package webhooks

import (
	"context"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var cfapplog = logf.Log.WithName("cfapp-validate")

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-workloads-cloudfoundry-org-v1alpha1-cfapp,mutating=false,failurePolicy=fail,sideEffects=None,groups=workloads.cloudfoundry.org,resources=cfapps,verbs=create;update,versions=v1alpha1,name=vcfapp.kb.io,admissionReviewVersions={v1,v1beta1}

type CFAppValidation struct {
	Client  client.Client
	decoder *admission.Decoder
}

func (v *CFAppValidation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-workloads-cloudfoundry-org-v1alpha1-cfapp", &webhook.Admission{Handler: v})
	return nil
}

func (v *CFAppValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	cfapplog.Info("Validate", "name", req.Name)
	return admission.Allowed("")
}

// InjectDecoder injects the decoder.
func (v *CFAppValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
