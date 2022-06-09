package webhook

import (
	"context"
	"fmt"
	"net/http"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/webhook/diff"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/lager"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type LRPResourceValidator struct {
	logger           lager.Logger
	decoder          *admission.Decoder
	lrpMutableFields []string
}

func NewLRPResourceValidator(logger lager.Logger, decoder *admission.Decoder) *LRPResourceValidator {
	return &LRPResourceValidator{
		logger:  logger,
		decoder: decoder,
		lrpMutableFields: []string{
			"Image",
			"Instances",
		},
	}
}

func (v *LRPResourceValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	updatedLRP := &eiriniv1.LRP{}

	err := v.decoder.DecodeRaw(req.Object, updatedLRP)
	if err != nil {
		return errorResponse("Error decoding object %s: %s", req.Object.String(), err.Error())
	}

	originalLRP := &eiriniv1.LRP{}

	err = v.decoder.DecodeRaw(req.OldObject, originalLRP)
	if err != nil {
		return errorResponse("Error decoding old object %s: %s", req.OldObject.String(), err.Error())
	}

	diffReport := diff.CompareLRPSpecs(&updatedLRP.Spec, &originalLRP.Spec, v.lrpMutableFields...)
	if diffReport != "" {
		return errorResponse("Changing immutable fields not allowed: %s", diffReport)
	}

	return admission.Response{
		AdmissionResponse: admissionv1.AdmissionResponse{
			Allowed: true,
		},
	}
}

func errorResponse(messageFormat string, formatArgs ...interface{}) admission.Response {
	return admission.Response{
		AdmissionResponse: admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  "Failure",
				Message: fmt.Sprintf(messageFormat, formatArgs...),
				Reason:  metav1.StatusReasonBadRequest,
				Code:    http.StatusBadRequest,
			},
		},
	}
}
