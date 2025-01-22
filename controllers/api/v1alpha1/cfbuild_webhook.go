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

// log is for logging in this package.
var cfbuildlog = logf.Log.WithName("cfbuild-resource")

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-cfbuild,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfbuilds,verbs=create;update,versions=v1alpha1,name=mcfbuild.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFBuildDefaulter struct{}

func NewCFBuildDefaulter() *CFBuildDefaulter {
	return &CFBuildDefaulter{}
}

func (d *CFBuildDefaulter) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&CFBuild{}).
		WithDefaulter(d).
		Complete()
}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *CFBuildDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	cfBuild := obj.(*CFBuild)
	cfbuildlog.V(1).Info("mutating Webhook for CFBuild", "name", cfBuild.Name)
	buildLabels := cfBuild.GetLabels()

	buildLabels = tools.SetMapValue(buildLabels, CFAppGUIDLabelKey, cfBuild.Spec.AppRef.Name)
	buildLabels = tools.SetMapValue(buildLabels, CFPackageGUIDLabelKey, cfBuild.Spec.PackageRef.Name)

	cfBuild.SetLabels(buildLabels)

	return nil
}
