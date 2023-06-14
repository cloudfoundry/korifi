package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type FinalizerDescriptor struct {
	FinalizerName string
	SetPolicy     func(unstructured.Unstructured) bool
}

func Always(_ unstructured.Unstructured) bool {
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
	var unstructuredObj unstructured.Unstructured

	if err := r.decoder.Decode(req, &unstructuredObj); err != nil {
		finalizerlog.Error(err, "failed to decode req object")
		return admission.Errored(http.StatusBadRequest, err)
	}

	var partialObj metav1.PartialObjectMetadata
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, &partialObj)
	if err != nil {
		finalizerlog.Error(err, "failed to convert unstructured to partialObj")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	origMarshalled, err := json.Marshal(partialObj)
	if err != nil {
		finalizerlog.Error(err, "failed to marshall partialObj")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	objKind := unstructuredObj.GetObjectKind().GroupVersionKind().Kind
	if finalizer, hasFinalizer := r.resourceTypeToFinalizerNameRegistry[objKind]; hasFinalizer {
		if !finalizer.SetPolicy(unstructuredObj) {
			return admission.Allowed(fmt.Sprintf("not applicable to %s %s/%s", objKind, unstructuredObj.GetNamespace(), unstructuredObj.GetName()))
		}

		if controllerutil.AddFinalizer(&partialObj, finalizer.FinalizerName) {
			finalizerlog.Info("added finalizer on object",
				"kind", unstructuredObj.GetObjectKind().GroupVersionKind().Kind,
				"namespace", unstructuredObj.GetNamespace(),
				"name", unstructuredObj.GetName())
		}
	} else {
		finalizerlog.Info("no finalizer registered for " + objKind)
	}

	marshalled, err := json.Marshal(partialObj)
	if err != nil {
		finalizerlog.Error(err, "failed to marshall updated partialObj")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(origMarshalled, marshalled)
}
