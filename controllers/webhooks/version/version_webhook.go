package version

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-all-version,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cforgs;cfspaces;builderinfos;cfdomains;cfserviceinstances;cfapps;cfpackages;cftasks;cfprocesses;cfbuilds;cfroutes;cfservicebindings;taskworkloads;appworkloads;buildworkloads,verbs=create;update,versions=v1alpha1,name=mcfversion.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

import (
	"context"
	"encoding/json"
	"net/http"

	"code.cloudfoundry.org/korifi/version"
	corev1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type VersionWebhook struct {
	version string
	decoder *admission.Decoder
}

func NewVersionWebhook(version string) *VersionWebhook {
	return &VersionWebhook{version: version}
}

var versionlog = logf.Log.WithName("version-webhook")

func (r *VersionWebhook) SetupWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register("/mutate-korifi-cloudfoundry-org-v1alpha1-all-version", &admission.Webhook{
		Handler: r,
	})
}

func (r *VersionWebhook) InjectDecoder(d *admission.Decoder) error {
	r.decoder = d
	return nil
}

func (r *VersionWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	versionlog.Info("adding-version")

	var obj metav1.PartialObjectMetadata

	if err := r.decoder.Decode(req, &obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	switch req.Operation {
	case corev1.Create:
		return r.setVersion(ctx, obj, r.version)
	case corev1.Update:
		return r.resetVersion(ctx, obj, req)
	default:
		return admission.Denied("we only accept create/update")
	}
}

func (r *VersionWebhook) setVersion(ctx context.Context, obj metav1.PartialObjectMetadata, ver string) admission.Response {
	origMarshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	anns := obj.GetAnnotations()
	if anns == nil {
		anns = map[string]string{}
	}
	anns[version.KorifiCreationVersionKey] = r.version
	obj.SetAnnotations(anns)

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(origMarshalled, marshalled)
}

func (r *VersionWebhook) resetVersion(ctx context.Context, obj metav1.PartialObjectMetadata, req admission.Request) admission.Response {
	if _, ok := obj.Annotations[version.KorifiCreationVersionKey]; ok {
		return admission.Allowed("already set")
	}

	var oldObj metav1.PartialObjectMetadata
	if err := r.decoder.DecodeRaw(req.OldObject, &oldObj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if oldVersion, ok := oldObj.Annotations[version.KorifiCreationVersionKey]; ok {
		return r.setVersion(ctx, obj, oldVersion)
	}

	return admission.Allowed("no old version")
}
