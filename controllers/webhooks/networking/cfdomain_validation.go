/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package networking

import (
	"context"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"

	admissionv1 "k8s.io/api/admission/v1"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	DomainDecodingErrorType  = "DomainDecodingError"
	DuplicateDomainErrorType = "DuplicateDomainError"
)

// log is for logging in this package.
var log = logf.Log.WithName("domain-validation")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfdomain,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfdomains,verbs=create;update,versions=v1alpha1,name=vcfdomain.korifi.cloudfoundry.org,admissionReviewVersions=v1

type CFDomainValidation struct {
	client  client.Client
	decoder *admission.Decoder
}

func NewCFDomainValidation(client client.Client) *CFDomainValidation {
	return &CFDomainValidation{
		client: client,
	}
}

func (v *CFDomainValidation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-korifi-cloudfoundry-org-v1alpha1-cfdomain", &webhook.Admission{Handler: v})
	return nil
}

func (v *CFDomainValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	var domain korifiv1alpha1.CFDomain
	if req.Operation != admissionv1.Create {
		return admission.Allowed("")
	}

	err := v.decoder.Decode(req, &domain)
	if err != nil { // untested
		errMessage := "Error while decoding CFDomain object"
		log.Error(err, errMessage)
		return admission.Denied(webhooks.ValidationError{Type: DomainDecodingErrorType, Message: errMessage}.Marshal())
	}

	isOverlapping, err := v.domainIsOverlapping(ctx, domain.Spec.Name)
	if err != nil {
		validationError := webhooks.ValidationError{
			Type:    webhooks.UnknownErrorType,
			Message: err.Error(),
		}
		return admission.Denied(validationError.Marshal())
	}

	if isOverlapping {
		return admission.Denied(webhooks.ValidationError{Type: DuplicateDomainErrorType, Message: "Overlapping domain exists"}.Marshal())
	}

	return admission.Allowed("")
}

func (v *CFDomainValidation) domainIsOverlapping(ctx context.Context, domainName string) (bool, error) {
	var existingDomainList korifiv1alpha1.CFDomainList
	err := v.client.List(ctx, &existingDomainList)
	if err != nil {
		return true, err
	}

	domainElements := strings.Split(domainName, ".")

	for _, existingDomain := range existingDomainList.Items {
		existingDomainElements := strings.Split(existingDomain.Spec.Name, ".")
		if isSubDomain(domainElements, existingDomainElements) {
			return true, nil
		}
	}

	return false, nil
}

func isSubDomain(domainElements, existingDomainElements []string) bool {
	var shorterSlice, longerSlice *[]string

	if len(domainElements) < len(existingDomainElements) {
		shorterSlice = &domainElements
		longerSlice = &existingDomainElements
	} else {
		shorterSlice = &existingDomainElements
		longerSlice = &domainElements
	}

	offset := len(*longerSlice) - len(*shorterSlice)

	for i := len(*shorterSlice) - 1; i >= 0; i-- {
		if (*shorterSlice)[i] != (*longerSlice)[i+offset] {
			return false
		}
	}
	return true
}

func (v *CFDomainValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
