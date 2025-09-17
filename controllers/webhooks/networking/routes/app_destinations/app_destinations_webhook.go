package routes

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-route-appdestinations,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfroutes,verbs=create;update,versions=v1alpha1,name=mcfrouteappdestinations.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/PaesslerAG/jsonpath"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var appdestinationlog = logf.Log.WithName("route-app-destinations-webhook")

type RouteAppDestinationsWebhook struct {
	decoder admission.Decoder
}

func NewRouteAppDestinationsWebhook() *RouteAppDestinationsWebhook {
	return &RouteAppDestinationsWebhook{}
}

func (r *RouteAppDestinationsWebhook) SetupWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register("/mutate-korifi-cloudfoundry-org-v1alpha1-route-appdestinations", &admission.Webhook{
		Handler: r,
	})
	r.decoder = admission.NewDecoder(mgr.GetScheme())
}

func (r *RouteAppDestinationsWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	var unstructuredObj unstructured.Unstructured

	if err := r.decoder.Decode(req, &unstructuredObj); err != nil {
		appdestinationlog.Error(err, "failed to decode req object")
		return admission.Errored(http.StatusBadRequest, err)
	}

	var partialObj metav1.PartialObjectMetadata
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, &partialObj)
	if err != nil {
		appdestinationlog.Error(err, "failed to convert unstructured to partialObj")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	origMarshalled, err := json.Marshal(partialObj)
	if err != nil {
		appdestinationlog.Error(err, "failed to marshall partialObj")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	data, err := jsonpath.Get("$.spec.destinations[*].appRef.name", unstructuredObj.Object)
	if err != nil {
		appdestinationlog.Error(err, "failed to get appRef names from route")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	marshalledData, err := json.Marshal(data)
	if err != nil {
		appdestinationlog.Error(err, "failed to marshall data")
		return admission.Errored(http.StatusInternalServerError, err)

	}

	// Unmarshalling here is done due to `data` cannot be casted directly to []string, it is of type []interface{}
	var destArr []string
	err = json.Unmarshal(marshalledData, &destArr)
	if err != nil {
		appdestinationlog.Error(err, "failed to unmarshal data")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	partialObj.SetAnnotations(tools.SetMapValue(partialObj.GetAnnotations(), korifiv1alpha1.CFRouteAppGuidsAnnotationKey, strings.Join(destArr, "\n")))

	marshalled, err := json.Marshal(partialObj)
	if err != nil {
		appdestinationlog.Error(err, "failed to marshall updated partialObj")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(origMarshalled, marshalled)
}
