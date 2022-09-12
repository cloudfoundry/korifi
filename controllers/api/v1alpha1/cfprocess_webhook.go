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

	runtime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// log is for logging in this package.
var cfprocesslog = logf.Log.WithName("cfprocess-resource")

type CFProcessDefaulter struct{}

func NewCFProcessDefaulter() *CFProcessDefaulter {
	return &CFProcessDefaulter{}
}

func (d *CFProcessDefaulter) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&CFProcess{}).
		WithDefaulter(d).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-cfprocess,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfprocesses,verbs=create;update,versions=v1alpha1,name=mcfprocess.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

func (d *CFProcessDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	process := obj.(*CFProcess)
	cfprocesslog.Info("Mutating CFProcess webhook handler", "name", process.Name)

	processLabels := process.GetLabels()

	if processLabels == nil {
		processLabels = make(map[string]string)
	}

	processLabels[CFProcessGUIDLabelKey] = process.Name
	processLabels[CFProcessTypeLabelKey] = process.Spec.ProcessType
	processLabels[CFAppGUIDLabelKey] = process.Spec.AppRef.Name

	process.SetLabels(processLabels)

	return nil
}
