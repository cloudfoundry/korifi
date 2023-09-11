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

package workloads

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var (
	cfpackagelog         = logf.Log.WithName("cftask-resource")
	appTypeToPackageType = map[v1alpha1.LifecycleType]v1alpha1.PackageType{
		"buildpack": "bits",
		"docker":    "docker",
	}
)

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfpackage,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfpackages,verbs=create;update,versions=v1alpha1,name=vcfpackage.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFPackageValidator struct {
	client client.Client
}

var _ webhook.CustomValidator = &CFPackageValidator{}

func NewCFPackageValidator() *CFPackageValidator {
	return &CFPackageValidator{}
}

func (v *CFPackageValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	v.client = mgr.GetClient()

	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.CFPackage{}).
		WithValidator(v).
		Complete()
}

var _ webhook.CustomValidator = &CFPackageValidator{}

func (v *CFPackageValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cfPackage, ok := obj.(*v1alpha1.CFPackage)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFPackage but got a %T", obj))
	}

	cfpackagelog.V(1).Info("validate package creation", "namespace", cfPackage.Namespace, "name", cfPackage.Name)

	cfApp := &v1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfPackage.Namespace,
			Name:      cfPackage.Spec.AppRef.Name,
		},
	}
	err := v.client.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)
	if err != nil {
		return nil, webhooks.ValidationError{
			Type:    "ReferencedAppDoesNotExistError",
			Message: fmt.Sprintf("referenced CFApp %q does not exist", cfPackage.Spec.AppRef.Name),
		}.ExportJSONError()
	}

	if cfPackage.Spec.Type != appTypeToPackageType[cfApp.Spec.Lifecycle.Type] {
		return nil, webhooks.ValidationError{
			Type:    "InvalidPackageTypeError",
			Message: fmt.Sprintf("cannot create %s package for a %s app", cfPackage.Spec.Type, cfApp.Spec.Lifecycle.Type),
		}.ExportJSONError()
	}

	return nil, nil
}

func (v *CFPackageValidator) ValidateUpdate(ctx context.Context, oldObj runtime.Object, obj runtime.Object) (admission.Warnings, error) {
	newCFPackage, ok := obj.(*v1alpha1.CFPackage)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFPackage but got a %T", obj))
	}

	if !newCFPackage.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

	cfpackagelog.V(1).Info("validate task update", "namespace", newCFPackage.Namespace, "name", newCFPackage.Name)

	oldCFPackage, ok := oldObj.(*v1alpha1.CFPackage)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFPackage but got a %T", oldObj))
	}

	if newCFPackage.Spec.Type != oldCFPackage.Spec.Type {
		return nil, webhooks.ValidationError{
			Type:    ImmutableFieldModificationErrorType,
			Message: fmt.Sprintf("package %s:%s Spec.Type is immutable", newCFPackage.Namespace, newCFPackage.Name),
		}.ExportJSONError()
	}

	return nil, nil
}

func (v *CFPackageValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
