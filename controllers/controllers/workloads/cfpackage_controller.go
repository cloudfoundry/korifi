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

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/image"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	cfPackageFinalizer       string = "korifi.cloudfoundry.org/cfPackageController"
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

// CFPackageReconciler reconciles a CFPackage object
type CFPackageReconciler struct {
	k8sClient             client.Client
	imageDeleter          ImageDeleter
	packageCleaner        PackageCleaner
	scheme                *runtime.Scheme
	packageRepoSecretName string
	log                   logr.Logger
}

func NewCFPackageReconciler(
	client client.Client,
	imageDeleter ImageDeleter,
	packageCleaner PackageCleaner,
	scheme *runtime.Scheme,
	packageRepoSecretName string,
	log logr.Logger,
) *CFPackageReconciler {
	return &CFPackageReconciler{
		k8sClient:             client,
		imageDeleter:          imageDeleter,
		packageCleaner:        packageCleaner,
		scheme:                scheme,
		packageRepoSecretName: packageRepoSecretName,
		log:                   log,
	}
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfpackages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfpackages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfpackages/finalizers,verbs=get;update;patch

func (r *CFPackageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.log.WithValues("namespace", req.Namespace, "name", req.Name)

	cfPackage := new(korifiv1alpha1.CFPackage)
	err := r.k8sClient.Get(ctx, req.NamespacedName, cfPackage)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Info("unable to fetch CFPackage", "reason", err)
		return ctrl.Result{}, err
	}

	if !cfPackage.GetDeletionTimestamp().IsZero() {
		return r.finalize(ctx, log, cfPackage)
	}

	err = k8s.AddFinalizer(ctx, log, r.k8sClient, cfPackage, cfPackageFinalizer)
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

	origPkg := cfPackage.DeepCopy()

	err = controllerutil.SetControllerReference(&cfApp, cfPackage, r.scheme)
	if err != nil {
		log.Info("unable to set owner reference on CFPackage", "reason", err)
		return ctrl.Result{}, err
	}

	err = r.k8sClient.Patch(ctx, cfPackage, client.MergeFrom(origPkg))
	if err != nil {
		r.log.Info("failed to patch package", "reason", err)
		return ctrl.Result{}, fmt.Errorf("failed to patch package: %w", err)
	}

	origPkg = cfPackage.DeepCopy()

	meta.SetStatusCondition(&cfPackage.Status.Conditions, metav1.Condition{
		Type:               InitializedConditionType,
		Status:             metav1.ConditionTrue,
		Reason:             "Initialized",
		ObservedGeneration: cfPackage.Generation,
	})

	readyCondition := metav1.ConditionFalse
	readyReason := "Initialized"
	if cfPackage.Spec.Source.Registry.Image != "" {
		readyCondition = metav1.ConditionTrue
		readyReason = "SourceImageSet"
	}
	meta.SetStatusCondition(&cfPackage.Status.Conditions, metav1.Condition{
		Type:               StatusConditionReady,
		Status:             readyCondition,
		Reason:             readyReason,
		ObservedGeneration: cfPackage.Generation,
	})

	if err = r.packageCleaner.Clean(ctx, types.NamespacedName{
		Namespace: cfPackage.Namespace,
		Name:      cfPackage.Spec.AppRef.Name,
	}); err != nil {
		log.Info("failed deleting older packages", "reason", err)
	}

	err = r.k8sClient.Status().Patch(ctx, cfPackage, client.MergeFrom(origPkg))
	if err != nil {
		r.log.Info("failed to patch package status", "reason", err)
		return ctrl.Result{}, fmt.Errorf("failed to patch package status: %w", err)
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
		}, cfPackage.Spec.Source.Registry.Image, cfPackage.Name); err != nil {
			log.Info("failed to delete image", "reason", err)
		}
	}

	origPkg := cfPackage.DeepCopy()
	if controllerutil.RemoveFinalizer(cfPackage, cfPackageFinalizer) {
		err := r.k8sClient.Patch(ctx, cfPackage, client.MergeFrom(origPkg))
		if err != nil {
			r.log.Info("failed to patch package", "reason", err)
			return ctrl.Result{}, fmt.Errorf("failed to patch package: %w", err)
		}
		log.Info("finalizer removed")
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFPackageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFPackage{}).
		Complete(r)
}
