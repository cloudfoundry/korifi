package common_labels

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-controllers-common-labels,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfapps;cfbuilds;cfdomains;cforgs;cfpackages;cfprocesss;cfroutes;cfsecuritygroups;cfservicebindings;cfservicebrokers;cfserviceinstances;cfservices;cfservices;cfspaces;cftasks,verbs=create;update,versions=v1alpha1,name=mcfcommonlabels.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	admissionv1 "k8s.io/api/admission/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type CommonLabelsWebhook struct {
	decoder admission.Decoder
}

func NewWebhook() *CommonLabelsWebhook {
	return &CommonLabelsWebhook{}
}

func (r *CommonLabelsWebhook) SetupWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register("/mutate-korifi-cloudfoundry-org-v1alpha1-controllers-common-labels", &admission.Webhook{
		Handler: r,
	})
	r.decoder = admission.NewDecoder(mgr.GetScheme())
}

func (r *CommonLabelsWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	var obj metav1.PartialObjectMetadata

	if err := r.decoder.Decode(req, &obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	origMarshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	if req.Operation == admissionv1.Create {
		obj.SetLabels(tools.SetMapValue(obj.GetLabels(), korifiv1alpha1.CreatedAtLabelKey, time.Now().Format(korifiv1alpha1.LabelDateFormat)))
	}

	if req.Operation == admissionv1.Update {
		obj.SetLabels(tools.SetMapValue(obj.GetLabels(), korifiv1alpha1.UpdatedAtLabelKey, time.Now().Format(korifiv1alpha1.LabelDateFormat)))
	}

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(origMarshalled, marshalled)
}
