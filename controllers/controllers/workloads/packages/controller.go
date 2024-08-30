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

package packages

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/image"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	InitializedConditionType string = "Initialized"
)

//counterfeiter:generate -o fake -fake-name ImageDeleter . ImageDeleter

type ImageDeleter interface {
	Delete(ctx context.Context, creds image.Creds, imageRef string, tagsToDelete ...string) error
}

//counterfeiter:generate -o fake -fake-name PackageCleaner . PackageCleaner

type PackageCleaner interface {
	Clean(ctx context.Context, app types.NamespacedName) error
}

type Reconciler struct {
	k8sClient              client.Client
	scheme                 *runtime.Scheme
	imageDeleter           ImageDeleter
	packageCleaner         PackageCleaner
	packageRepoSecretNames []string
	log                    logr.Logger
}

func NewReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	imageDeleter ImageDeleter,
	packageCleaner PackageCleaner,
	packageRepoSecretNames []string,
) *k8s.PatchingReconciler[korifiv1alpha1.CFPackage, *korifiv1alpha1.CFPackage] {
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFPackage, *korifiv1alpha1.CFPackage](log, client, &Reconciler{
		k8sClient:              client,
		scheme:                 scheme,
		log:                    log,
		imageDeleter:           imageDeleter,
		packageCleaner:         packageCleaner,
		packageRepoSecretNames: packageRepoSecretNames,
	})
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFPackage{})
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfpackages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfpackages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfpackages/finalizers,verbs=get;update;patch

func (r *Reconciler) ReconcileResource(ctx context.Context, cfPackage *korifiv1alpha1.CFPackage) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	cfPackage.Status.ObservedGeneration = cfPackage.Generation
	log.V(1).Info("set observed generation", "generation", cfPackage.Status.ObservedGeneration)

	if !cfPackage.GetDeletionTimestamp().IsZero() {
		return r.finalize(ctx, cfPackage)
	}

	var cfApp korifiv1alpha1.CFApp
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfPackage.Spec.AppRef.Name, Namespace: cfPackage.Namespace}, &cfApp)
	if err != nil {
		log.Info("error when fetching CFApp", "reason", err)
		return ctrl.Result{}, err
	}

	err = controllerutil.SetControllerReference(&cfApp, cfPackage, r.scheme)
	if err != nil {
		log.Info("unable to set owner reference on CFPackage", "reason", err)
		return ctrl.Result{}, err
	}

	meta.SetStatusCondition(&cfPackage.Status.Conditions, metav1.Condition{
		Type:               InitializedConditionType,
		Status:             metav1.ConditionTrue,
		Reason:             "Initialized",
		ObservedGeneration: cfPackage.Generation,
	})

	defer func() {
		if err = r.packageCleaner.Clean(ctx, types.NamespacedName{
			Namespace: cfPackage.Namespace,
			Name:      cfPackage.Spec.AppRef.Name,
		}); err != nil {
			log.Info("failed deleting older packages", "reason", err)
		}
	}()

	if cfPackage.Spec.Source.Registry.Image == "" {
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("Initialized").WithNoRequeue()
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) finalize(ctx context.Context, cfPackage *korifiv1alpha1.CFPackage) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("finalize")

	if !controllerutil.ContainsFinalizer(cfPackage, korifiv1alpha1.CFPackageFinalizerName) {
		return ctrl.Result{}, nil
	}

	if cfPackage.Spec.Type != "docker" && cfPackage.Spec.Source.Registry.Image != "" {
		if err := r.imageDeleter.Delete(ctx, image.Creds{
			Namespace:   cfPackage.Namespace,
			SecretNames: r.packageRepoSecretNames,
		}, cfPackage.Spec.Source.Registry.Image, cfPackage.Name); err != nil {
			log.Info("failed to delete image", "reason", err)
		}
	}

	if controllerutil.RemoveFinalizer(cfPackage, korifiv1alpha1.CFPackageFinalizerName) {
		log.V(1).Info("finalizer removed")
	}

	return ctrl.Result{}, nil
}
