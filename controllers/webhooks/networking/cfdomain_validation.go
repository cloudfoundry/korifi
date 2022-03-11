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

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	admissionv1 "k8s.io/api/admission/v1"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var log = logf.Log.WithName("domain-validation")

//+kubebuilder:webhook:path=/validate-networking-cloudfoundry-org-v1alpha1-cfdomain,mutating=false,failurePolicy=fail,sideEffects=None,groups=networking.cloudfoundry.org,resources=cfdomains,verbs=create;update,versions=v1alpha1,name=vcfdomain.kb.io,admissionReviewVersions=v1

//counterfeiter:generate -o fake -fake-name Client sigs.k8s.io/controller-runtime/pkg/client.Client

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
	mgr.GetWebhookServer().Register("/validate-networking-cloudfoundry-org-v1alpha1-cfdomain", &webhook.Admission{Handler: v})
	return nil
}

func (v *CFDomainValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	var domain networkingv1alpha1.CFDomain
	if req.Operation != admissionv1.Create {
		return admission.Allowed("")
	}

	err := v.decoder.Decode(req, &domain)
	if err != nil {
		errMessage := "Error while decoding CFDomain object"
		log.Error(err, errMessage)
		return admission.Denied(errMessage)
	}

	isOverlapping, err := v.domainIsOverlapping(ctx, domain.Spec.Name)
	if err != nil {
		return admission.Denied(err.Error())
	}

	if isOverlapping {
		return admission.Denied(webhooks.DuplicateDomainError.Marshal())
	}

	return admission.Allowed("")
}

func (v *CFDomainValidation) domainIsOverlapping(ctx context.Context, domainName string) (bool, error) {
	var existingDomainList networkingv1alpha1.CFDomainList
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
