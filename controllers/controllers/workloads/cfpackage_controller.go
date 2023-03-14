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

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/image"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const cfPackageFinalizer string = "korifi.cloudfoundry.org/cfPackageController"

//counterfeiter:generate -o fake -fake-name ImageDeleter . ImageDeleter

type ImageDeleter interface {
	Delete(ctx context.Context, creds image.Creds, imageRef string) error
}

// CFPackageReconciler reconciles a CFPackage object
type CFPackageReconciler struct {
	k8sClient             client.Client
	imageDeleter          ImageDeleter
	scheme                *runtime.Scheme
	packageRepoSecretName string
	log                   logr.Logger
}

func NewCFPackageReconciler(
	client client.Client,
	imageDeleter ImageDeleter,
	scheme *runtime.Scheme,
	packageRepoSecretName string,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.CFPackage, *korifiv1alpha1.CFPackage] {
	pkgReconciler := CFPackageReconciler{
		k8sClient:             client,
		imageDeleter:          imageDeleter,
		scheme:                scheme,
		packageRepoSecretName: packageRepoSecretName,
		log:                   log,
	}

	return k8s.NewPatchingReconciler[korifiv1alpha1.CFPackage, *korifiv1alpha1.CFPackage](log, client, &pkgReconciler)
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfpackages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfpackages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfpackages/finalizers,verbs=get;update;patch

func (r *CFPackageReconciler) ReconcileResource(ctx context.Context, cfPackage *korifiv1alpha1.CFPackage) (ctrl.Result, error) {
	log := r.log.WithValues("namespace", cfPackage.Namespace, "name", cfPackage.Name)

	if !cfPackage.GetDeletionTimestamp().IsZero() {
		return r.finalize(ctx, log, cfPackage)
	}

	err := k8s.AddFinalizer(ctx, log, r.k8sClient, cfPackage, cfPackageFinalizer)
	if err != nil {
		log.Error(err, "Error adding finalizer")
		return ctrl.Result{}, err
	}

	var cfApp korifiv1alpha1.CFApp
	err = r.k8sClient.Get(ctx, types.NamespacedName{Name: cfPackage.Spec.AppRef.Name, Namespace: cfPackage.Namespace}, &cfApp)
	if err != nil {
		log.Info("error when fetching CFApp", "reason", err)
		return ctrl.Result{}, err
	}

	err = controllerutil.SetControllerReference(&cfApp, cfPackage, r.scheme)
	if err != nil {
		log.Info("unable to set owner reference on CFPackage", "reason", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFPackageReconciler) finalize(ctx context.Context, log logr.Logger, cfPackage *korifiv1alpha1.CFPackage) (ctrl.Result, error) {
	log = log.WithName("finalize")

	if !controllerutil.ContainsFinalizer(cfPackage, cfPackageFinalizer) {
		return ctrl.Result{}, nil
	}

	if cfPackage.Spec.Source.Registry.Image != "" {
		if err := r.imageDeleter.Delete(ctx, image.Creds{
			Namespace:  cfPackage.Namespace,
			SecretName: r.packageRepoSecretName,
		}, cfPackage.Spec.Source.Registry.Image); err != nil {
			log.Info("failed to delete image", "reason", err)
		}
	}

	if controllerutil.RemoveFinalizer(cfPackage, cfPackageFinalizer) {
		log.Info("finalizer removed")
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFPackageReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFPackage{})
}
