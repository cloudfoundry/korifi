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
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/build"
	"code.cloudfoundry.org/korifi/tools/image"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type ImageConfigGetter interface {
	Config(context.Context, image.Creds, string) (image.Config, error)
}

func NewCFDockerBuildReconciler(
	k8sClient client.Client,
	buildCleaner build.BuildCleaner,
	imageConfigGetter ImageConfigGetter,
	scheme *runtime.Scheme,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.CFBuild, *korifiv1alpha1.CFBuild] {
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFBuild, *korifiv1alpha1.CFBuild](
		log,
		k8sClient,
		build.NewCFBuildReconciler(
			log,
			k8sClient,
			scheme,
			buildCleaner,
			&dockerBuildReconciler{
				k8sClient:         k8sClient,
				imageConfigGetter: imageConfigGetter,
			},
		))
}

type dockerBuildReconciler struct {
	k8sClient         client.Client
	imageConfigGetter ImageConfigGetter
}

func (r *dockerBuildReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFBuild{}).
		WithEventFilter(predicate.NewPredicateFuncs(dockerBuildFilter))
}

func dockerBuildFilter(object client.Object) bool {
	cfBuild, ok := object.(*korifiv1alpha1.CFBuild)
	if !ok {
		return false
	}

	return cfBuild.Spec.Lifecycle.Type == korifiv1alpha1.LifecycleType("docker")
}

// +kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds/finalizers,verbs=update
func (r *dockerBuildReconciler) ReconcileBuild(
	ctx context.Context,
	cfBuild *korifiv1alpha1.CFBuild,
	cfApp *korifiv1alpha1.CFApp,
	cfPackage *korifiv1alpha1.CFPackage,
) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)
	succeededStatus := shared.GetConditionOrSetAsUnknown(&cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType, cfBuild.Generation)
	if succeededStatus != metav1.ConditionUnknown {
		log.Info("build status indicates completion", "status", succeededStatus)
		return ctrl.Result{}, nil
	}

	secretNames := []string{}
	for _, secretRef := range cfPackage.Spec.Source.Registry.ImagePullSecrets {
		secretNames = append(secretNames, secretRef.Name)
	}

	imageConfig, err := r.imageConfigGetter.Config(
		ctx,
		image.Creds{
			Namespace:   cfPackage.Namespace,
			SecretNames: secretNames,
		},
		cfPackage.Spec.Source.Registry.Image,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
		Type:               korifiv1alpha1.StagingConditionType,
		Status:             metav1.ConditionFalse,
		Reason:             "BuildNotRunning",
		ObservedGeneration: cfBuild.Generation,
	})

	if isRoot(imageConfig.User) {
		meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
			Type:   korifiv1alpha1.SucceededConditionType,
			Status: metav1.ConditionFalse,
			Reason: "BuildFailed",
			Message: fmt.Sprintf(
				"Image %q is configured to run as the root user. That is insecure on Kubernetes and therefore not supported by Korifi.",
				cfPackage.Spec.Source.Registry.Image,
			),
			ObservedGeneration: cfBuild.Generation,
		})

		return ctrl.Result{}, nil
	}

	meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
		Type:               korifiv1alpha1.SucceededConditionType,
		Status:             metav1.ConditionTrue,
		Reason:             "BuildSucceeded",
		ObservedGeneration: cfBuild.Generation,
	})

	cfBuild.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{
		Registry: cfPackage.Spec.Source.Registry,
		Ports:    imageConfig.ExposedPorts,
	}

	return ctrl.Result{}, nil
}

func isRoot(user string) bool {
	user = strings.Split(user, ":")[0]
	return user == "" || user == "root" || user == "0"
}
