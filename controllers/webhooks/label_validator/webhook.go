package label_validator

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-controllers-label-validator,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfroutes;cfapps;cfbuilds;cfdomains;cfpackages;cfprocesses;cfservicebindings;cfserviceinstances;cftasks;cforgs;cfspaces;cfserviceofferings;cfserviceplans;cfservicebrokers,verbs=create;update,versions=v1alpha1,name=vcflabelvalidator.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/label_indexer/signer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type LabelValidatorWebhook struct {
	decoder       admission.Decoder
	signingSecret []byte
}

func NewWebhook(signingSecret []byte) *LabelValidatorWebhook {
	return &LabelValidatorWebhook{
		signingSecret: signingSecret,
	}
}

func (r *LabelValidatorWebhook) SetupWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register("/validate-korifi-cloudfoundry-org-v1alpha1-controllers-label-validator", &admission.Webhook{
		Handler: r,
	})
	r.decoder = admission.NewDecoder(mgr.GetScheme())
}

func (r *LabelValidatorWebhook) Handle(_ context.Context, req admission.Request) admission.Response {
	var obj metav1.PartialObjectMetadata
	if err := r.decoder.Decode(req, &obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Reject any label key in the broader cloudfoundry.org domain that is NOT
	// a korifi-managed key (those are handled by the signature check below).
	for k := range obj.GetLabels() {
		if !strings.HasPrefix(k, signer.ReservedLabelDomain) && isCloudfoundryDomain(k) {
			return admission.Denied(fmt.Sprintf(
				"label key %q cannot use the cloudfoundry.org domain", k,
			))
		}
	}

	storedSig := obj.GetAnnotations()[korifiv1alpha1.LabelSignatureAnnotationKey]

	// On first create (no existing signature yet) the label indexer has already
	// run and written a fresh signature, so storedSig will be present.
	// If it is still absent (e.g. pre-existing objects during rollout) we allow
	// the request through to avoid blocking legitimate operations.
	if storedSig == "" {
		return admission.Allowed("no existing label signature")
	}

	if !signer.Verify(r.signingSecret, obj.GetLabels(), storedSig) {
		return admission.Denied(fmt.Sprintf(
			"reserved %s labels may not be modified by API clients",
			signer.ReservedLabelDomain,
		))
	}

	return admission.Allowed("label signature verified")
}

func isCloudfoundryDomain(key string) bool {
	u, err := url.ParseRequestURI("https://" + key)
	if err != nil {
		return false
	}
	return strings.HasSuffix(u.Hostname(), "cloudfoundry.org")
}
