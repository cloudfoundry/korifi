package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type FinalizerDescriptor struct {
	FinalizerName string
	SetPolicy     func(client.Object) bool
}

func Always(_ client.Object) bool {
	return true
}

type FinalizerWebhook struct {
	decoder                             *admission.Decoder
	resourceTypeToFinalizerNameRegistry map[string]FinalizerDescriptor
}

func NewFinalizerWebhook(resourceTypeToFinalizerNameRegistry map[string]FinalizerDescriptor) *FinalizerWebhook {
	return &FinalizerWebhook{resourceTypeToFinalizerNameRegistry: resourceTypeToFinalizerNameRegistry}
}

var finalizerlog = logf.Log.WithName("finalizer-webhook")

func (r *FinalizerWebhook) SetupWebhookWithManager(mgr ctrl.Manager) {
	r.decoder = admission.NewDecoder(mgr.GetScheme())
}

func (r *FinalizerWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	var obj metav1.PartialObjectMetadata

	if err := r.decoder.Decode(req, &obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	origMarshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	objKind := obj.GetObjectKind().GroupVersionKind().Kind
	if finalizer, hasFinalizer := r.resourceTypeToFinalizerNameRegistry[objKind]; hasFinalizer {
		if !finalizer.SetPolicy(&obj) {
			return admission.Allowed(fmt.Sprintf("not applicable to %s %s/%s", objKind, obj.GetNamespace(), obj.GetName()))
		}

		if controllerutil.AddFinalizer(&obj, finalizer.FinalizerName) {
			finalizerlog.Info("added finalizer on object",
				"kind", obj.GetObjectKind().GroupVersionKind().Kind,
				"namespace", obj.GetNamespace(),
				"name", obj.GetName())
		}
	} else {
		finalizerlog.Info("no finalizer registered for " + objKind)
	}

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(origMarshalled, marshalled)
}
