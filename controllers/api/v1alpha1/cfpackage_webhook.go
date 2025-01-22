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

package v1alpha1

import (
	"context"

	"code.cloudfoundry.org/korifi/tools"
	ctrl "sigs.k8s.io/controller-runtime"

	runtime "k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// log is for logging in this package.
var cfpackagelog = logf.Log.WithName("cfpackage-resource")

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-cfpackage,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfpackages,verbs=create;update,versions=v1alpha1,name=mcfpackage.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFPackageDefaulter struct{}

func NewCFPackageDefaulter() *CFPackageDefaulter {
	return &CFPackageDefaulter{}
}

func (d *CFPackageDefaulter) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&CFPackage{}).
		WithDefaulter(d).
		Complete()
}

func (r *CFPackageDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	cfPackage := obj.(*CFPackage)
	cfpackagelog.V(1).Info("mutating CFPackage webhook handler", "name", cfPackage.Name)

	cfPackage.SetLabels(tools.SetMapValue(cfPackage.GetLabels(), CFAppGUIDLabelKey, cfPackage.Spec.AppRef.Name))

	return nil
}
