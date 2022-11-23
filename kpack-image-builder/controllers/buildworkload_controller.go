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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/kpack-image-builder/config"
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
	kpackReadyConditionType  = "Ready"
	clusterBuilderKind       = "ClusterBuilder"
	clusterBuilderAPIVersion = "kpack.io/v1alpha2"
	kpackServiceAccount      = "kpack-service-account"
	BuildWorkloadLabelKey    = "korifi.cloudfoundry.org/build-workload-name"
	kpackReconcilerName      = "kpack-image-builder"
)

//counterfeiter:generate -o fake -fake-name RegistryAuthFetcher . RegistryAuthFetcher
type RegistryAuthFetcher func(ctx context.Context, namespace string) (remote.Option, error)

func NewRegistryAuthFetcher(privilegedK8sClient k8sclient.Interface) RegistryAuthFetcher {
	return func(ctx context.Context, namespace string) (remote.Option, error) {
		keychain, err := k8schain.New(ctx, privilegedK8sClient, k8schain.Options{
			Namespace:          namespace,
			ServiceAccountName: kpackServiceAccount,
		})
		if err != nil {
			return nil, fmt.Errorf("error in keychainFactory.KeychainForSecretRef: %w", err)
		}

		return remote.WithAuthFromKeychain(keychain), nil
	}
}

//counterfeiter:generate -o fake -fake-name ImageProcessFetcher . ImageProcessFetcher
type ImageProcessFetcher func(imageRef string, credsOption remote.Option, transport remote.Option) ([]korifiv1alpha1.ProcessType, []int32, error)

func NewBuildWorkloadReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	config *config.ControllerConfig,
	registryAuthFetcher RegistryAuthFetcher,
	registryCAPath string,
	imageProcessFetcher ImageProcessFetcher,
) *k8s.PatchingReconciler[korifiv1alpha1.BuildWorkload, *korifiv1alpha1.BuildWorkload] {
	buildWorkloadReconciler := BuildWorkloadReconciler{
		k8sClient:           c,
		scheme:              scheme,
		log:                 log,
		controllerConfig:    config,
		registryAuthFetcher: registryAuthFetcher,
		registryCAPath:      registryCAPath,
		imageProcessFetcher: imageProcessFetcher,
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
	registryCAPath      string
	imageProcessFetcher ImageProcessFetcher
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
			r.log.Info("Kpack image requirements for buildWorkload are not met", "guid", buildWorkload.Name, "reason", err)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, r.createKpackImageAndUpdateStatus(ctx, buildWorkload)
	}

	var kpackImage buildv1alpha2.Image
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), &kpackImage)
	if err != nil {
		r.log.Error(err, "Error when fetching Kpack Image")
		// Ignore Image NotFound errors to account for eventual consistency
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	kpackReadyStatusCondition := kpackImage.Status.GetCondition(kpackReadyConditionType)
	if kpackReadyStatusCondition.IsFalse() {
		meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.SucceededConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "BuildFailed",
			Message: "Check build log output",
		})
	} else if kpackReadyStatusCondition.IsTrue() {
		meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.SucceededConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  "BuildSucceeded",
			Message: "Image built successfully",
		})

		serviceAccountName := kpackServiceAccount
		serviceAccountLookupKey := types.NamespacedName{Name: serviceAccountName, Namespace: buildWorkload.Namespace}
		foundServiceAccount := corev1.ServiceAccount{}
		err = r.k8sClient.Get(ctx, serviceAccountLookupKey, &foundServiceAccount)
		if err != nil {
			r.log.Error(err, "Error when fetching kpack ServiceAccount")
			return ctrl.Result{}, err
		}

		buildWorkload.Status.Droplet, err = r.generateDropletStatus(ctx, &kpackImage, foundServiceAccount.ImagePullSecrets)
		if err != nil {
			r.log.Error(err, "Error when compiling the DropletStatus")
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
	serviceAccountName := kpackServiceAccount
	kpackImageName := buildWorkload.Name
	kpackImageNamespace := buildWorkload.Namespace
	desiredKpackImage := buildv1alpha2.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kpackImageName,
			Namespace: kpackImageNamespace,
			Labels: map[string]string{
				BuildWorkloadLabelKey: buildWorkload.Name,
			},
		},
		Spec: buildv1alpha2.ImageSpec{
			Tag: r.controllerConfig.DropletRepository,
			Builder: corev1.ObjectReference{
				Kind:       clusterBuilderKind,
				Name:       r.controllerConfig.ClusterBuilderName,
				APIVersion: clusterBuilderAPIVersion,
			},
			ServiceAccountName: serviceAccountName,
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
		r.log.Error(err, "failed to set OwnerRef on Kpack Image")
		return err
	}

	err = r.createKpackImageIfNotExists(ctx, desiredKpackImage)
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

func (r *BuildWorkloadReconciler) createKpackImageIfNotExists(ctx context.Context, desiredKpackImage buildv1alpha2.Image) error {
	var foundKpackImage buildv1alpha2.Image
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(&desiredKpackImage), &foundKpackImage)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = r.k8sClient.Create(ctx, &desiredKpackImage)
			if err != nil {
				r.log.Error(err, "Error when creating kpack image")
				return err
			}
		} else {
			r.log.Error(err, "Error when checking if kpack image exists")
			return err
		}
	}
	return nil
}

func (r *BuildWorkloadReconciler) generateDropletStatus(ctx context.Context, kpackImage *buildv1alpha2.Image, imagePullSecrets []corev1.LocalObjectReference) (*korifiv1alpha1.BuildDropletStatus, error) {
	imageRef := kpackImage.Status.LatestImage

	credentials, err := r.registryAuthFetcher(ctx, kpackImage.Namespace)
	if err != nil {
		r.log.Error(err, "Error when fetching registry credentials for Droplet image")
		return nil, err
	}

	transport, err := configureTransport(r.registryCAPath)
	if err != nil {
		r.log.Error(err, "Error when configuring http transport for Droplet image")
		return nil, err
	}

	// Use the credentials to get the values of Ports and ProcessTypes
	dropletProcessTypes, dropletPorts, err := r.imageProcessFetcher(imageRef, credentials, transport)
	if err != nil {
		r.log.Error(err, "Error when compiling droplet image details")
		return nil, err
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

func configureTransport(caCertPath string) (remote.Option, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}

	if caCertPath != "" {
		var pemCerts []byte
		if pemCerts, err = os.ReadFile(caCertPath); err != nil {
			return nil, err
		} else if ok := pool.AppendCertsFromPEM(pemCerts); !ok {
			return nil, errors.New("failed to append k8s cert bundle to cert pool")
		}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    pool,
	}

	return remote.WithTransport(transport), nil
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

func filterBuildWorkloads(object client.Object) bool {
	buildWorkload, ok := object.(*korifiv1alpha1.BuildWorkload)
	if !ok {
		return true
	}

	// Only reconcile buildworkloads that have their Spec.BuilderName matching this builder
	return buildWorkload.Spec.BuilderName == kpackReconcilerName
}
