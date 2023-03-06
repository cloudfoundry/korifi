/*
Copyright 2022.

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

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

package controllers

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn/k8schain"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	corev1alpha1 "github.com/pivotal/kpack/pkg/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	clusterBuilderKind       = "ClusterBuilder"
	clusterBuilderAPIVersion = "kpack.io/v1alpha2"
	BuildWorkloadLabelKey    = "korifi.cloudfoundry.org/build-workload-name"
	kpackReconcilerName      = "kpack-image-builder"
)

//counterfeiter:generate -o fake -fake-name RepositoryCreator . RepositoryCreator

type RepositoryCreator interface {
	CreateRepository(ctx context.Context, name string) error
}

//counterfeiter:generate -o fake -fake-name RegistryAuthFetcher . RegistryAuthFetcher

type RegistryAuthFetcher func(ctx context.Context, namespace string) (remote.Option, error)

func NewRegistryAuthFetcher(privilegedK8sClient k8sclient.Interface, serviceAccount string) RegistryAuthFetcher {
	return func(ctx context.Context, namespace string) (remote.Option, error) {
		keychain, err := k8schain.New(ctx, privilegedK8sClient, k8schain.Options{
			Namespace:          namespace,
			ServiceAccountName: serviceAccount,
		})
		if err != nil {
			return nil, fmt.Errorf("error in keychainFactory.KeychainForSecretRef: %w", err)
		}

		return remote.WithAuthFromKeychain(keychain), nil
	}
}

//counterfeiter:generate -o fake -fake-name ImageProcessFetcher . ImageProcessFetcher
type ImageProcessFetcher func(imageRef string, credsOption remote.Option) ([]korifiv1alpha1.ProcessType, []int32, error)

func NewBuildWorkloadReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	config *config.ControllerConfig,
	registryAuthFetcher RegistryAuthFetcher,
	imageProcessFetcher ImageProcessFetcher,
	imageRepoPrefix string,
	imageRepoCreator RepositoryCreator,
) *k8s.PatchingReconciler[korifiv1alpha1.BuildWorkload, *korifiv1alpha1.BuildWorkload] {
	buildWorkloadReconciler := BuildWorkloadReconciler{
		k8sClient:           c,
		scheme:              scheme,
		log:                 log,
		controllerConfig:    config,
		registryAuthFetcher: registryAuthFetcher,
		imageProcessFetcher: imageProcessFetcher,
		imageRepoPrefix:     imageRepoPrefix,
		imageRepoCreator:    imageRepoCreator,
	}
	return k8s.NewPatchingReconciler[korifiv1alpha1.BuildWorkload, *korifiv1alpha1.BuildWorkload](log, c, &buildWorkloadReconciler)
}

// BuildWorkloadReconciler reconciles a BuildWorkload object
type BuildWorkloadReconciler struct {
	k8sClient           client.Client
	scheme              *runtime.Scheme
	log                 logr.Logger
	controllerConfig    *config.ControllerConfig
	registryAuthFetcher RegistryAuthFetcher
	imageProcessFetcher ImageProcessFetcher
	imageRepoPrefix     string
	imageRepoCreator    RepositoryCreator
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads/status,verbs=get;patch

//+kubebuilder:rbac:groups=kpack.io,resources=images,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups=kpack.io,resources=images/status,verbs=get;patch

//+kubebuilder:rbac:groups="",resources=serviceaccounts;secrets,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups="",resources=serviceaccounts/status;secrets/status,verbs=get

func (r *BuildWorkloadReconciler) ReconcileResource(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload) (ctrl.Result, error) {
	if len(buildWorkload.Spec.Buildpacks) > 0 {
		// Specifying buildpacks is not supported
		meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.SucceededConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "InvalidBuildpacks",
			Message: `Only buildpack auto-detection is supported. Specifying buildpacks is not allowed.`,
		})

		return ctrl.Result{}, nil
	}

	succeededStatus := meta.FindStatusCondition(buildWorkload.Status.Conditions, korifiv1alpha1.SucceededConditionType)

	if succeededStatus != nil && succeededStatus.Status != metav1.ConditionUnknown {
		return ctrl.Result{}, nil
	}

	if succeededStatus == nil {
		err := r.ensureKpackImageRequirements(ctx, buildWorkload)
		if err != nil {
			r.log.Info("kpack image requirements for buildWorkload are not met", "guid", buildWorkload.Name, "reason", err)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, r.createKpackImageAndUpdateStatus(ctx, buildWorkload)
	}

	var kpackImage buildv1alpha2.Image
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), &kpackImage)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		} else {
			r.log.Info("error when fetching Kpack Image", "reason", err)
			return ctrl.Result{}, err
		}
	}

	kpackReadyStatusCondition := kpackImage.Status.GetCondition(corev1alpha1.ConditionReady)
	kpackBuilderReadyStatusCondition := kpackImage.Status.GetCondition(buildv1alpha2.ConditionBuilderReady)
	if kpackReadyStatusCondition.IsFalse() {
		meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.SucceededConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "BuildFailed",
			Message: "Check build log output",
		})
	} else if kpackBuilderReadyStatusCondition.IsFalse() {
		meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.SucceededConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "BuilderNotReady",
			Message: "Check ClusterBuilder",
		})
	} else if kpackReadyStatusCondition.IsTrue() {
		meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.SucceededConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  "BuildSucceeded",
			Message: "Image built successfully",
		})

		foundServiceAccount := corev1.ServiceAccount{}
		err = r.k8sClient.Get(ctx, types.NamespacedName{
			Namespace: buildWorkload.Namespace,
			Name:      r.controllerConfig.BuilderServiceAccount,
		}, &foundServiceAccount)
		if err != nil {
			r.log.Info("error when fetching kpack ServiceAccount", "reason", err)
			return ctrl.Result{}, err
		}

		buildWorkload.Status.Droplet, err = r.generateDropletStatus(ctx, &kpackImage, foundServiceAccount.ImagePullSecrets)
		if err != nil {
			r.log.Info("error when compiling the DropletStatus", "reason", err)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *BuildWorkloadReconciler) ensureKpackImageRequirements(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload) error {
	for _, secret := range buildWorkload.Spec.Source.Registry.ImagePullSecrets {
		err := r.k8sClient.Get(ctx, types.NamespacedName{Namespace: buildWorkload.Namespace, Name: secret.Name}, &corev1.Secret{})
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *BuildWorkloadReconciler) createKpackImageAndUpdateStatus(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload) error {
	appGUID := buildWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey]
	kpackImageName := buildWorkload.Name
	kpackImageNamespace := buildWorkload.Namespace
	kpackImageTag := r.repositoryRef(appGUID)
	desiredKpackImage := buildv1alpha2.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kpackImageName,
			Namespace: kpackImageNamespace,
			Labels: map[string]string{
				BuildWorkloadLabelKey: buildWorkload.Name,
			},
		},
		Spec: buildv1alpha2.ImageSpec{
			Tag: kpackImageTag,
			Builder: corev1.ObjectReference{
				Kind:       clusterBuilderKind,
				Name:       r.controllerConfig.ClusterBuilderName,
				APIVersion: clusterBuilderAPIVersion,
			},
			ServiceAccountName: r.controllerConfig.BuilderServiceAccount,
			Source: corev1alpha1.SourceConfig{
				Registry: &corev1alpha1.Registry{
					Image:            buildWorkload.Spec.Source.Registry.Image,
					ImagePullSecrets: buildWorkload.Spec.Source.Registry.ImagePullSecrets,
				},
			},
			Build: &buildv1alpha2.ImageBuild{
				Services: buildWorkload.Spec.Services,
				Env:      buildWorkload.Spec.Env,
			},
		},
	}

	err := controllerutil.SetOwnerReference(buildWorkload, &desiredKpackImage, r.scheme)
	if err != nil {
		r.log.Info("failed to set OwnerRef on Kpack Image", "reason", err)
		return err
	}

	err = r.createKpackImageIfNotExists(ctx, desiredKpackImage, appGUID)
	if err != nil {
		return err
	}

	meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
		Type:    korifiv1alpha1.SucceededConditionType,
		Status:  metav1.ConditionUnknown,
		Reason:  "BuildRunning",
		Message: "Waiting for image build to complete",
	})

	return nil
}

func (r *BuildWorkloadReconciler) createKpackImageIfNotExists(ctx context.Context, desiredKpackImage buildv1alpha2.Image, appGUID string) error {
	var foundKpackImage buildv1alpha2.Image
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(&desiredKpackImage), &foundKpackImage)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err = r.imageRepoCreator.CreateRepository(ctx, r.repositoryRef(appGUID)); err != nil {
				r.log.Info("failed to create image repository", "reason", err)
				return err
			}

			err = r.k8sClient.Create(ctx, &desiredKpackImage)
			if err != nil {
				r.log.Info("error when creating kpack image", "reason", err)
				return err
			}
		} else {
			r.log.Info("error when checking if kpack image exists", "reason", err)
			return err
		}
	}
	return nil
}

func (r *BuildWorkloadReconciler) generateDropletStatus(ctx context.Context, kpackImage *buildv1alpha2.Image, imagePullSecrets []corev1.LocalObjectReference) (*korifiv1alpha1.BuildDropletStatus, error) {
	imageRef := kpackImage.Status.LatestImage

	credentials, err := r.registryAuthFetcher(ctx, kpackImage.Namespace)
	if err != nil {
		return nil, fmt.Errorf("error when fetching registry credentials for Droplet image: %w", err)
	}

	// Use the credentials to get the values of Ports and ProcessTypes
	dropletProcessTypes, dropletPorts, err := r.imageProcessFetcher(imageRef, credentials)
	if err != nil {
		return nil, fmt.Errorf("error when compiling droplet image details: %w", err)
	}

	return &korifiv1alpha1.BuildDropletStatus{
		Registry: korifiv1alpha1.Registry{
			Image:            imageRef,
			ImagePullSecrets: imagePullSecrets,
		},

		Stack: kpackImage.Status.LatestStack,

		ProcessTypes: dropletProcessTypes,
		Ports:        dropletPorts,
	}, nil
}

func (r *BuildWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.BuildWorkload{}).
		Watches(
			&source.Kind{Type: new(buildv1alpha2.Image)},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				var requests []reconcile.Request
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      obj.GetLabels()[BuildWorkloadLabelKey],
						Namespace: obj.GetNamespace(),
					},
				})
				return requests
			})).
		WithEventFilter(predicate.NewPredicateFuncs(filterBuildWorkloads))
}

func (r *BuildWorkloadReconciler) repositoryRef(appGUID string) string {
	return r.imageRepoPrefix + appGUID + "-droplets"
}

func filterBuildWorkloads(object client.Object) bool {
	buildWorkload, ok := object.(*korifiv1alpha1.BuildWorkload)
	if !ok {
		return true
	}

	// Only reconcile buildworkloads that have their Spec.BuilderName matching this builder
	return buildWorkload.Spec.BuilderName == kpackReconcilerName
}
