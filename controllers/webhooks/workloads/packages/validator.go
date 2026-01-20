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

package packages

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	validationwebhook "code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var (
	cfpackagelog = logf.Log.WithName("cftask-resource")
)

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfpackage,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfpackages,verbs=update,versions=v1alpha1,name=vcfpackage.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type Validator struct {
	client client.Client
}

var _ admission.Validator[*korifiv1alpha1.CFPackage] = &Validator{}

func NewValidator() *Validator {
	return &Validator{}
}

func (v *Validator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	v.client = mgr.GetClient()

	return ctrl.NewWebhookManagedBy(mgr, &korifiv1alpha1.CFPackage{}).
		WithValidator(v).
		Complete()
}

func (v *Validator) ValidateCreate(_ context.Context, _ *korifiv1alpha1.CFPackage) (admission.Warnings, error) {
	return nil, nil
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldPackage, newPackage *korifiv1alpha1.CFPackage) (admission.Warnings, error) {
	if !newPackage.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

	cfpackagelog.V(1).Info("validate task update", "namespace", newPackage.Namespace, "name", newPackage.Name)

	if newPackage.Spec.Type != oldPackage.Spec.Type {
		return nil, validationwebhook.ValidationError{
			Type:    webhooks.ImmutableFieldModificationErrorType,
			Message: fmt.Sprintf("package %s:%s Spec.Type is immutable", newPackage.Namespace, newPackage.Name),
		}.ExportJSONError()
	}

	return nil, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ *korifiv1alpha1.CFPackage) (admission.Warnings, error) {
	return nil, nil
}
