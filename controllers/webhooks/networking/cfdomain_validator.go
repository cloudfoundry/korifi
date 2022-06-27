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
	"fmt"
	"strings"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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

type CFDomainValidator struct {
	client client.Client
}

var _ webhook.CustomValidator = &CFDomainValidator{}

func NewCFDomainValidator(client client.Client) *CFDomainValidator {
	return &CFDomainValidator{
		client: client,
	}
}

func (v *CFDomainValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.CFDomain{}).
		WithValidator(v).
		Complete()
}

func (v *CFDomainValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	domain, ok := obj.(*korifiv1alpha1.CFDomain)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFDomain but got a %T", obj))
	}

	isOverlapping, err := v.domainIsOverlapping(ctx, domain.Spec.Name)
	if err != nil {
		log.Error(err, "Error checking for overlapping domain")
		return webhooks.ValidationError{
			Type:    webhooks.UnknownErrorType,
			Message: webhooks.UnknownErrorMessage,
		}.ExportJSONError()
	}

	if isOverlapping {
		return webhooks.ValidationError{
			Type:    DuplicateDomainErrorType,
			Message: "Overlapping domain exists",
		}.ExportJSONError()
	}

	return nil
}

func (v *CFDomainValidator) ValidateUpdate(ctx context.Context, oldObj runtime.Object, obj runtime.Object) error {
	oldDomain, ok := oldObj.(*korifiv1alpha1.CFDomain)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFDomain but got a %T", obj))
	}

	domain, ok := obj.(*korifiv1alpha1.CFDomain)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFDomain but got a %T", obj))
	}

	if oldDomain.Spec.Name != domain.Spec.Name {
		return webhooks.ValidationError{
			Type:    webhooks.ImmutableFieldErrorType,
			Message: fmt.Sprintf(webhooks.ImmutableFieldErrorMessageTemplate, "CFDomain.Spec.Name"),
		}.ExportJSONError()
	}

	return nil
}

func (v *CFDomainValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}

func (v *CFDomainValidator) domainIsOverlapping(ctx context.Context, domainName string) (bool, error) {
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
