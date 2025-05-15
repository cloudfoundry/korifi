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
	runtime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// log is for logging in this package.
var cfroutelog = logf.Log.WithName("cfroute-resource")

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-cfroute,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfroutes,verbs=create;update,versions=v1alpha1,name=mcfroute.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFRouteDefaulter struct{}

func NewCFRouteDefaulter() *CFRouteDefaulter {
	return &CFRouteDefaulter{}
}

func (d *CFRouteDefaulter) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&CFRoute{}).
		WithDefaulter(d).
		Complete()
}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *CFRouteDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	cfRoute := obj.(*CFRoute)
	cfroutelog.V(1).Info("mutating CFRoute webhook handler", "name", cfRoute.Name)
	routeLabels := cfRoute.GetLabels()
	routeLabels = tools.SetMapValue(routeLabels, CFRouteGUIDLabelKey, cfRoute.Name)
	cfRoute.SetLabels(routeLabels)

	return nil
}
