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
var cfserviceplanlog = logf.Log.WithName("cfserviceplan-resource")

func (r *CFServicePlan) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-cfserviceplan,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfserviceplans,verbs=create;update,versions=v1alpha1,name=mcfserviceplan.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &CFServicePlan{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *CFServicePlan) Default() {
	cfserviceplanlog.V(1).Info("mutating CFServicePlan webhook handler", "name", r.Name)
	if r.Spec.Visibility.Type == "" {
		r.Spec.Visibility.Type = "admin"
	}
}