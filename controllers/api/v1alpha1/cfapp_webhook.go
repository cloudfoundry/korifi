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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	CFAppFinalizerName = "cfApp.korifi.cloudfoundry.org"
)

// log is for logging in this package.
var cfapplog = logf.Log.WithName("cfapp-resource")

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-cfapp,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfapps,verbs=create;update,versions=v1alpha1,name=mcfapp.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFAppDefaulter struct{}

func NewCFAppDefaulter() *CFAppDefaulter {
	return &CFAppDefaulter{}
}

func (d *CFAppDefaulter) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&CFApp{}).
		WithDefaulter(d).
		Complete()
}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *CFAppDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	cfApp := obj.(*CFApp)
	cfapplog.V(1).Info("mutating CFApp webhook handler", "name", cfApp.Name)

	r.defaultLabels(cfApp)
	r.defaultAnnotations(cfApp)

	return nil
}

func (r *CFAppDefaulter) defaultLabels(cfApp *CFApp) {
	labels := cfApp.GetLabels()
	labels = tools.SetMapValue(labels, CFAppGUIDLabelKey, cfApp.Name)
	labels = tools.SetMapValue(labels, GUIDLabelKey, cfApp.Name)
	labels = tools.SetMapValue(labels, CFAppDisplayNameKey, tools.EncodeValueToSha224(cfApp.Spec.DisplayName))
	cfApp.SetLabels(labels)
}

func (r *CFAppDefaulter) defaultAnnotations(cfApp *CFApp) {
	appAnnotations := cfApp.GetAnnotations()
	_, hasRevAnnotation := appAnnotations[CFAppRevisionKey]

	if !hasRevAnnotation {
		appAnnotations = tools.SetMapValue(appAnnotations, CFAppRevisionKey, CFAppDefaultRevision)
	}
	cfApp.SetAnnotations(appAnnotations)
}
