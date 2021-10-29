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
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var cfpackagelog = logf.Log.WithName("cfpackage-resource")

func (r *CFPackage) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-workloads-cloudfoundry-org-v1alpha1-cfpackage,mutating=true,failurePolicy=fail,sideEffects=None,groups=workloads.cloudfoundry.org,resources=cfpackages,verbs=create;update,versions=v1alpha1,name=mcfpackage.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &CFPackage{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *CFPackage) Default() {
	cfpackagelog.Info("Mutating CFPackage webhook handler", "name", r.Name)
	packageLabels := r.ObjectMeta.GetLabels()
	if packageLabels == nil {
		packageLabels = make(map[string]string)
	}
	packageLabels[CFAppGUIDLabelKey] = r.Spec.AppRef.Name
	r.ObjectMeta.SetLabels(packageLabels)
}
