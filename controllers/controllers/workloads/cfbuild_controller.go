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

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	config "code.cloudfoundry.org/cf-k8s-controllers/config/base"

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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	kpackReadyConditionType   = "Ready"
	clusterBuilderKind        = "ClusterBuilder"
	clusterBuilderAPIVersion  = "kpack.io/v1alpha2"
	kpackServiceAccountSuffix = "-kpack-service-account"
	cfKpackClusterBuilderName = "cf-kpack-cluster-builder"
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
			ServiceAccount: kpackServiceAccountName(namespace),
		})
		if err != nil {
			return nil, fmt.Errorf("error in keychainFactory.KeychainForSecretRef: %w", err)
		}

		return remote.WithAuthFromKeychain(keychain), nil
	}
}

func kpackServiceAccountName(namespace string) string {
	return namespace + kpackServiceAccountSuffix
}

//counterfeiter:generate -o fake -fake-name ImageProcessFetcher . ImageProcessFetcher
type ImageProcessFetcher func(imageRef string, credsOption remote.Option) ([]workloadsv1alpha1.ProcessType, []int32, error)

// CFBuildReconciler reconciles a CFBuild object
type CFBuildReconciler struct {
	Client              CFClient
	Scheme              *runtime.Scheme
	Log                 logr.Logger
	ControllerConfig    *config.ControllerConfig
	RegistryAuthFetcher RegistryAuthFetcher
	ImageProcessFetcher ImageProcessFetcher
}

//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfbuilds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfbuilds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfbuilds/finalizers,verbs=update

//+kubebuilder:rbac:groups=kpack.io,resources=images,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kpack.io,resources=images/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kpack.io,resources=images/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=serviceaccounts;secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=serviceaccounts/status;secrets/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CFBuild object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *CFBuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cfBuild workloadsv1alpha1.CFBuild
	err := r.Client.Get(ctx, req.NamespacedName, &cfBuild)
	if err != nil {
		r.Log.Error(err, "Error when fetching CFBuild")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var cfApp workloadsv1alpha1.CFApp
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfBuild.Spec.AppRef.Name, Namespace: cfBuild.Namespace}, &cfApp)
	if err != nil {
		r.Log.Error(err, "Error when fetching CFApp")
		return ctrl.Result{}, err
	}

	var cfPackage workloadsv1alpha1.CFPackage
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfBuild.Spec.PackageRef.Name, Namespace: cfBuild.Namespace}, &cfPackage)
	if err != nil {
		r.Log.Error(err, "Error when fetching CFPackage")
		return ctrl.Result{}, err
	}

	stagingStatus := getConditionOrSetAsUnknown(&cfBuild.Status.Conditions, workloadsv1alpha1.StagingConditionType)
	succeededStatus := getConditionOrSetAsUnknown(&cfBuild.Status.Conditions, workloadsv1alpha1.SucceededConditionType)

	if stagingStatus == metav1.ConditionUnknown &&
		succeededStatus == metav1.ConditionUnknown {
		// Scenario: CFBuild newly created and all status conditions are unknown, it
		// Creates a KpackImage resource to trigger staging.
		// Updates status on CFBuild -> sets staging to True.
		err = r.createKpackImageAndUpdateStatus(ctx, &cfBuild, &cfApp, &cfPackage)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if stagingStatus == metav1.ConditionTrue &&
		succeededStatus == metav1.ConditionUnknown {
		// Scenario: CFBuild reconciles when Type staging is True and Type ready is False, it
		// Retrieves and Checks Kpack Image Status Condition for Type "Succeeded"
		// If NotFound error - Ignore and return
		// If Found, check Succeeded status condition
		// If Succeeded is True - Update Status Conditions and Droplet fields on CFBuild
		// If Succeeded is False - Update Status Conditions on CFBuild
		var kpackImage buildv1alpha2.Image
		err = r.Client.Get(ctx, types.NamespacedName{Name: cfBuild.Name, Namespace: cfBuild.Namespace}, &kpackImage)
		if err != nil {
			r.Log.Error(err, "Error when fetching Kpack Image")
			//Ignore Image NotFound errors to account for eventual consistency
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		kpackReadyStatusCondition := kpackImage.Status.GetCondition(kpackReadyConditionType)
		if kpackReadyStatusCondition.IsFalse() {
			// Set CFBuild status Conditions on local copy - Staging and Succeeded to False
			failureStatusConditionMessage := r.concatenateStrings(":", kpackReadyStatusCondition.Reason, kpackReadyStatusCondition.Message)
			setStatusConditionOnLocalCopy(&cfBuild.Status.Conditions, workloadsv1alpha1.StagingConditionType, metav1.ConditionFalse, "kpack", "kpack")
			setStatusConditionOnLocalCopy(&cfBuild.Status.Conditions, workloadsv1alpha1.SucceededConditionType, metav1.ConditionFalse, "kpack", failureStatusConditionMessage)
			if err := r.Client.Status().Update(ctx, &cfBuild); err != nil {
				r.Log.Error(err, "Error when updating CFBuild status")
				return ctrl.Result{}, err
			}
		} else if kpackReadyStatusCondition.IsTrue() {
			// Set CFBuild status Conditions on local copy- Staging to False and Succeeded to True
			setStatusConditionOnLocalCopy(&cfBuild.Status.Conditions, workloadsv1alpha1.StagingConditionType, metav1.ConditionFalse, "kpack", "kpack")
			setStatusConditionOnLocalCopy(&cfBuild.Status.Conditions, workloadsv1alpha1.SucceededConditionType, metav1.ConditionTrue, "kpack", "kpack")

			// try to find the ServiceAccount image pull secrets from the kpack service account
			serviceAccountName := kpackServiceAccountName(cfBuild.Namespace)
			serviceAccountLookupKey := types.NamespacedName{Name: serviceAccountName, Namespace: req.Namespace}
			foundServiceAccount := corev1.ServiceAccount{}
			err = r.Client.Get(ctx, serviceAccountLookupKey, &foundServiceAccount)
			if err != nil {
				r.Log.Error(err, "Error when fetching kpack ServiceAccount")
				return ctrl.Result{}, err
			}

			// Generate Droplet object using kpack Image and set it on CFBuild local copy
			cfBuild.Status.BuildDropletStatus, err = r.generateBuildDropletStatus(ctx, &kpackImage, foundServiceAccount.ImagePullSecrets)
			if err != nil {
				r.Log.Error(err, "Error when compiling the DropletStatus")
				return ctrl.Result{}, err
			}

			// Call Status().Update() tp push updates to the server
			if err := r.Client.Status().Update(ctx, &cfBuild); err != nil {
				r.Log.Error(err, "Error when updating CFBuild status")
				return ctrl.Result{}, err
			}
		}
	}
	return ctrl.Result{}, nil
}

func (r *CFBuildReconciler) createKpackImageAndUpdateStatus(ctx context.Context, cfBuild *workloadsv1alpha1.CFBuild, cfApp *workloadsv1alpha1.CFApp, cfPackage *workloadsv1alpha1.CFPackage) error {
	serviceAccountName := kpackServiceAccountName(cfBuild.Namespace)
	kpackImageTag := r.concatenateStrings("/", r.ControllerConfig.KpackImageTag, cfBuild.Name)
	kpackImageName := cfBuild.Name
	kpackImageNamespace := cfBuild.Namespace
	desiredKpackImage := buildv1alpha2.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kpackImageName,
			Namespace: kpackImageNamespace,
			Labels: map[string]string{
				workloadsv1alpha1.CFBuildGUIDLabelKey: cfBuild.Name,
				workloadsv1alpha1.CFAppGUIDLabelKey:   cfApp.Name,
			},
		},
		Spec: buildv1alpha2.ImageSpec{
			Tag: kpackImageTag,
			Builder: corev1.ObjectReference{
				Kind:       clusterBuilderKind,
				Name:       cfKpackClusterBuilderName,
				APIVersion: clusterBuilderAPIVersion,
			},
			ServiceAccountName: serviceAccountName,
			Source: corev1alpha1.SourceConfig{
				Registry: &corev1alpha1.Registry{
					Image:            cfPackage.Spec.Source.Registry.Image,
					ImagePullSecrets: cfPackage.Spec.Source.Registry.ImagePullSecrets,
				},
			},
		},
	}

	err := r.createKpackImageIfNotExists(ctx, desiredKpackImage)
	if err != nil {
		return err
	}

	setStatusConditionOnLocalCopy(&cfBuild.Status.Conditions, workloadsv1alpha1.StagingConditionType, metav1.ConditionTrue, "kpack", "kpack")

	// Update CFBuild record based on changes made to local copy
	if err := r.Client.Status().Update(ctx, cfBuild); err != nil {
		r.Log.Error(err, "Error when updating CFBuild status")
		return err
	}

	return nil
}

func (r *CFBuildReconciler) createKpackImageIfNotExists(ctx context.Context, desiredKpackImage buildv1alpha2.Image) error {
	var foundKpackImage buildv1alpha2.Image
	err := r.Client.Get(ctx, types.NamespacedName{Name: desiredKpackImage.Name, Namespace: desiredKpackImage.Namespace}, &foundKpackImage)
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

func (r *CFBuildReconciler) generateBuildDropletStatus(ctx context.Context, kpackImage *buildv1alpha2.Image, imagePullSecrets []corev1.LocalObjectReference) (*workloadsv1alpha1.BuildDropletStatus, error) {

	imageRef := kpackImage.Status.LatestImage
	//imagePullSecrets := kpackImage.Spec.Source.Registry.ImagePullSecrets

	// Use ImagePullSecrets to extract credentials with RegistryAuthFetcher
	// RegistryAuthFetcher func(ctx context.Context, imagePullSecrets []corev1.LocalObjectReference, namespace string) (remote.Option, error)
	credentials, err := r.RegistryAuthFetcher(ctx, kpackImage.Namespace)
	if err != nil {
		r.Log.Error(err, "Error when fetching registry credentials for Droplet image")
		return nil, err
	}

	// Use the credentials to get the values of Ports and ProcessTypes
	dropletProcessTypes, dropletPorts, err := r.ImageProcessFetcher(imageRef, credentials)
	if err != nil {
		r.Log.Error(err, "Error when compiling droplet image details")
		return nil, err
	}

	return &workloadsv1alpha1.BuildDropletStatus{
		Registry: workloadsv1alpha1.Registry{
			Image:            imageRef,
			ImagePullSecrets: imagePullSecrets,
		},

		Stack: kpackImage.Status.LatestStack,

		ProcessTypes: dropletProcessTypes,
		Ports:        dropletPorts,
	}, nil
}

func (r *CFBuildReconciler) concatenateStrings(separator string, text ...string) string {
	return strings.Join(text, separator)
}

// getConditionOrSetAsUnknown is a helper function that retrieves the value of the provided conditionType, like "Succeeded" and returns the value: "True", "False", or "Unknown"
// If the value is not present, the pointer to the list of conditions provided to the function is used to add an entry to the list of Conditions with a value of "Unknown" and "Unknown" is returned
func getConditionOrSetAsUnknown(conditions *[]metav1.Condition, conditionType string) metav1.ConditionStatus {
	conditionStatus := meta.FindStatusCondition(*conditions, conditionType)
	conditionStatusValue := metav1.ConditionUnknown
	if conditionStatus != nil {
		conditionStatusValue = conditionStatus.Status
	} else {
		// set local copy of CR condition to "unknown" because it had no value
		meta.SetStatusCondition(conditions, metav1.Condition{
			Type:    conditionType,
			Status:  metav1.ConditionUnknown,
			Reason:  "Unknown",
			Message: "Unknown",
		})
	}
	return conditionStatusValue
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFBuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadsv1alpha1.CFBuild{}).
		Watches(
			&source.Kind{Type: &buildv1alpha2.Image{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				var requests []reconcile.Request
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      obj.GetLabels()[workloadsv1alpha1.CFBuildGUIDLabelKey],
						Namespace: obj.GetNamespace(),
					}})
				return requests
			})).
		Complete(r)
}
