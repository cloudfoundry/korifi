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
var cfprocesslog = logf.Log.WithName("cfprocess-resource")

type CFProcessDefaulter struct {
	defaultMemoryMB    int64
	defaultDiskQuotaMB int64
}

func NewCFProcessDefaulter(defaultMemoryMB, defaultDiskQuotaMB int64) *CFProcessDefaulter {
	return &CFProcessDefaulter{
		defaultMemoryMB:    defaultMemoryMB,
		defaultDiskQuotaMB: defaultDiskQuotaMB,
	}
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

	d.defaultLabels(process)
	d.defaultResources(process)
	d.defaultInstances(process)
	d.defaultHealthCheck(process)

	return nil
}

func (d *CFProcessDefaulter) defaultLabels(process *CFProcess) {
	processLabels := process.GetLabels()

	if processLabels == nil {
		processLabels = make(map[string]string)
	}

	processLabels[CFProcessGUIDLabelKey] = process.Name
	processLabels[CFProcessTypeLabelKey] = process.Spec.ProcessType
	processLabels[CFAppGUIDLabelKey] = process.Spec.AppRef.Name

	process.SetLabels(processLabels)
}

func (d *CFProcessDefaulter) defaultResources(process *CFProcess) {
	if process.Spec.MemoryMB == 0 {
		process.Spec.MemoryMB = d.defaultMemoryMB
	}

	if process.Spec.DiskQuotaMB == 0 {
		process.Spec.DiskQuotaMB = d.defaultDiskQuotaMB
	}
}

func (d *CFProcessDefaulter) defaultInstances(process *CFProcess) {
	if process.Spec.DesiredInstances != nil {
		return
	}

	defaultInstances := 0
	if process.Spec.ProcessType == ProcessTypeWeb {
		defaultInstances = 1
	}
	process.Spec.DesiredInstances = tools.PtrTo(defaultInstances)
}

func (d *CFProcessDefaulter) defaultHealthCheck(process *CFProcess) {
	if process.Spec.HealthCheck.Type != "" {
		return
	}

	if process.Spec.ProcessType == ProcessTypeWeb {
		process.Spec.HealthCheck.Type = "port"
		return
	}

	process.Spec.HealthCheck.Type = "process"
}
