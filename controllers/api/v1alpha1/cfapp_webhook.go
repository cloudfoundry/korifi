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
var cfapplog = logf.Log.WithName("cfapp-resource")

func (r *CFApp) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-cfapp,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfapps,verbs=create;update,versions=v1alpha1,name=mcfapp.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &CFApp{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *CFApp) Default() {
	cfapplog.Info("Mutating CFApp webhook handler", "name", r.Name)
	r.SetLabels(r.defaultLabels(r.GetLabels()))
	r.SetAnnotations(r.defaultAnnotations(r.GetAnnotations()))
}

func (r *CFApp) defaultLabels(appLabels map[string]string) map[string]string {
	if appLabels == nil {
		appLabels = make(map[string]string)
	}
	appLabels[CFAppGUIDLabelKey] = r.Name

	return appLabels
}

func (r *CFApp) defaultAnnotations(appAnnotations map[string]string) map[string]string {
	if appAnnotations == nil {
		appAnnotations = make(map[string]string)
	}

	_, hasRevAnnotation := appAnnotations[CFAppRevisionKey]
	if !hasRevAnnotation {
		appAnnotations[CFAppRevisionKey] = CFAppRevisionKeyDefault
	}

	return appAnnotations
}
