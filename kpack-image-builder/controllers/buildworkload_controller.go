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
	"path"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/kpack-image-builder/config"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	corev1alpha1 "github.com/pivotal/kpack/pkg/apis/core/v1alpha1"
	"github.com/pivotal/kpack/pkg/dockercreds/k8sdockercreds"
	"github.com/pivotal/kpack/pkg/registry"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
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
		keychainFactory, err := k8sdockercreds.NewSecretKeychainFactory(privilegedK8sClient)
		if err != nil {
			return nil, fmt.Errorf("error in k8sdockercreds.NewSecretKeychainFactory: %w", err)
		}
		keychain, err := keychainFactory.KeychainForSecretRef(ctx, registry.SecretRef{
			Namespace:      namespace,
			ServiceAccount: kpackServiceAccount,
		})
		if err != nil {
			return nil, fmt.Errorf("error in keychainFactory.KeychainForSecretRef: %w", err)
		}

		return remote.WithAuthFromKeychain(keychain), nil
	}
}

//counterfeiter:generate -o fake -fake-name ImageProcessFetcher . ImageProcessFetcher
type ImageProcessFetcher func(imageRef string, credsOption remote.Option, transport remote.Option) ([]korifiv1alpha1.ProcessType, []int32, error)

func NewBuildWorkloadReconciler(c client.Client, scheme *runtime.Scheme, log logr.Logger, config *config.ControllerConfig, registryAuthFetcher RegistryAuthFetcher, registryCAPath string, imageProcessFetcher ImageProcessFetcher) *BuildWorkloadReconciler {
	return &BuildWorkloadReconciler{
		Client:              c,
		Scheme:              scheme,
		Log:                 log,
		ControllerConfig:    config,
		RegistryAuthFetcher: registryAuthFetcher,
		RegistryCAPath:      registryCAPath,
		ImageProcessFetcher: imageProcessFetcher,
	}
}

// BuildWorkloadReconciler reconciles a BuildWorkload object
type BuildWorkloadReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	Log                 logr.Logger
	ControllerConfig    *config.ControllerConfig
	RegistryAuthFetcher RegistryAuthFetcher
	RegistryCAPath      string
	ImageProcessFetcher ImageProcessFetcher
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads/finalizers,verbs=update

//+kubebuilder:rbac:groups=kpack.io,resources=images,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kpack.io,resources=images/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kpack.io,resources=images/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=serviceaccounts;secrets,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups="",resources=serviceaccounts/status;secrets/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *BuildWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	buildWorkload := new(korifiv1alpha1.BuildWorkload)
	err := r.Client.Get(ctx, req.NamespacedName, buildWorkload)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			r.Log.Error(err, "Error when fetching CFBuildWorkload")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if buildWorkload.Spec.ReconcilerName != kpackReconcilerName {
		// Stop reconciling since the buildWorkload.Spec.ReconcilerName does not match this builder
		return ctrl.Result{}, nil
	}

	if len(buildWorkload.Spec.Buildpacks) > 0 {
		// Specifying buildpacks is not supported
		failureStatusConditionMessage := `Only buildpack auto-detection is supported. Specifying buildpacks is not allowed.`
		setSucceededConditionOnLocalCopy(&buildWorkload.Status.Conditions, metav1.ConditionFalse, "InvalidBuildpacks", failureStatusConditionMessage)
		err = r.Client.Status().Update(ctx, buildWorkload)
		if err != nil {
			r.Log.Error(err, "Error when updating buildWorkload status")
		}
		return ctrl.Result{}, err
	}

	succeededStatus := meta.FindStatusCondition(buildWorkload.Status.Conditions, korifiv1alpha1.SucceededConditionType)

	if succeededStatus != nil && succeededStatus.Status != metav1.ConditionUnknown {
		return ctrl.Result{}, err
	}

	if succeededStatus == nil {
		err = r.ensureKpackImageRequirements(ctx, buildWorkload)
		if err != nil {
			r.Log.Info("Kpack image requirements for buildWorkload are not met", "guid", buildWorkload.Name, "reason", err)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, r.createKpackImageAndUpdateStatus(ctx, buildWorkload)
	}

	var kpackImage buildv1alpha2.Image
	err = r.Client.Get(ctx, client.ObjectKeyFromObject(buildWorkload), &kpackImage)
	if err != nil {
		r.Log.Error(err, "Error when fetching Kpack Image")
		// Ignore Image NotFound errors to account for eventual consistency
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	kpackReadyStatusCondition := kpackImage.Status.GetCondition(kpackReadyConditionType)
	if kpackReadyStatusCondition.IsFalse() {
		failureStatusConditionMessage := kpackReadyStatusCondition.Reason + ":" + kpackReadyStatusCondition.Message
		setSucceededConditionOnLocalCopy(&buildWorkload.Status.Conditions, metav1.ConditionFalse, "kpack", failureStatusConditionMessage)
		if err = r.Client.Status().Update(ctx, buildWorkload); err != nil {
			r.Log.Error(err, "Error when updating buildWorkload status")
			return ctrl.Result{}, err
		}
	} else if kpackReadyStatusCondition.IsTrue() {
		setSucceededConditionOnLocalCopy(&buildWorkload.Status.Conditions, metav1.ConditionTrue, "kpack", "kpack")

		serviceAccountName := kpackServiceAccount
		serviceAccountLookupKey := types.NamespacedName{Name: serviceAccountName, Namespace: req.Namespace}
		foundServiceAccount := corev1.ServiceAccount{}
		err = r.Client.Get(ctx, serviceAccountLookupKey, &foundServiceAccount)
		if err != nil {
			r.Log.Error(err, "Error when fetching kpack ServiceAccount")
			return ctrl.Result{}, err
		}

		buildWorkload.Status.Droplet, err = r.generateDropletStatus(ctx, &kpackImage, foundServiceAccount.ImagePullSecrets)
		if err != nil {
			r.Log.Error(err, "Error when compiling the DropletStatus")
			return ctrl.Result{}, err
		}

		if err := r.Client.Status().Update(ctx, buildWorkload); err != nil {
			r.Log.Error(err, "Error when updating CFBuild status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *BuildWorkloadReconciler) ensureKpackImageRequirements(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload) error {
	for _, secret := range buildWorkload.Spec.Source.Registry.ImagePullSecrets {
		err := r.Client.Get(ctx, types.NamespacedName{Namespace: buildWorkload.Namespace, Name: secret.Name}, &corev1.Secret{})
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *BuildWorkloadReconciler) createKpackImageAndUpdateStatus(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload) error {
	serviceAccountName := kpackServiceAccount
	kpackImageTag := path.Join(r.ControllerConfig.KpackImageTag, buildWorkload.Name)
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
			Tag: kpackImageTag,
			Builder: corev1.ObjectReference{
				Kind:       clusterBuilderKind,
				Name:       r.ControllerConfig.ClusterBuilderName,
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

	err := controllerutil.SetOwnerReference(buildWorkload, &desiredKpackImage, r.Scheme)
	if err != nil {
		r.Log.Error(err, "failed to set OwnerRef on Kpack Image")
		return err
	}

	err = r.createKpackImageIfNotExists(ctx, desiredKpackImage)
	if err != nil {
		return err
	}

	setSucceededConditionOnLocalCopy(&buildWorkload.Status.Conditions, metav1.ConditionUnknown, "Unknown", "Unknown")
	if err := r.Client.Status().Update(ctx, buildWorkload); err != nil {
		r.Log.Error(err, "Error when updating buildWorkload status")
		return err
	}

	return nil
}

func (r *BuildWorkloadReconciler) createKpackImageIfNotExists(ctx context.Context, desiredKpackImage buildv1alpha2.Image) error {
	var foundKpackImage buildv1alpha2.Image
	err := r.Client.Get(ctx, client.ObjectKeyFromObject(&desiredKpackImage), &foundKpackImage)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = r.Client.Create(ctx, &desiredKpackImage)
			if err != nil {
				r.Log.Error(err, "Error when creating kpack image")
				return err
			}
		} else {
			r.Log.Error(err, "Error when checking if kpack image exists")
			return err
		}
	}
	return nil
}

func setSucceededConditionOnLocalCopy(conditions *[]metav1.Condition, conditionStatus metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:    korifiv1alpha1.SucceededConditionType,
		Status:  conditionStatus,
		Reason:  reason,
		Message: message,
	})
}

func (r *BuildWorkloadReconciler) generateDropletStatus(ctx context.Context, kpackImage *buildv1alpha2.Image, imagePullSecrets []corev1.LocalObjectReference) (*korifiv1alpha1.BuildDropletStatus, error) {
	imageRef := kpackImage.Status.LatestImage

	credentials, err := r.RegistryAuthFetcher(ctx, kpackImage.Namespace)
	if err != nil {
		r.Log.Error(err, "Error when fetching registry credentials for Droplet image")
		return nil, err
	}

	transport, err := configureTransport(r.RegistryCAPath)
	if err != nil {
		r.Log.Error(err, "Error when configuring http transport for Droplet image")
		return nil, err
	}

	// Use the credentials to get the values of Ports and ProcessTypes
	dropletProcessTypes, dropletPorts, err := r.ImageProcessFetcher(imageRef, credentials, transport)
	if err != nil {
		r.Log.Error(err, "Error when compiling droplet image details")
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

// SetupWithManager sets up the controller with the Manager.
func (r *BuildWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
		Complete(r)
}
