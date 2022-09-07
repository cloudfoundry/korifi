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

package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/api/repositories/conditions"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/go-logr/logr"
	servicebindingv1beta1 "github.com/servicebinding/service-binding-controller/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	CFServiceBindingFinalizerName = "cfServiceBinding.korifi.cloudfoundry.org"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name VCAPServicesSecretBuilder . VCAPServicesSecretBuilder
type VCAPServicesSecretBuilder interface {
	BuildVCAPServicesEnvValue(context.Context, *korifiv1alpha1.CFApp) (string, error)
}

// CFServiceBindingReconciler reconciles a CFServiceBinding object
type CFServiceBindingReconciler struct {
	client.Client
	scheme  *runtime.Scheme
	log     logr.Logger
	builder VCAPServicesSecretBuilder
}

func NewCFServiceBindingReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger, builder VCAPServicesSecretBuilder) *CFServiceBindingReconciler {
	return &CFServiceBindingReconciler{Client: client, scheme: scheme, log: log, builder: builder}
}

const (
	BindingSecretAvailableCondition      = "BindingSecretAvailable"
	VCAPServicesSecretAvailableCondition = "VCAPServicesSecretAvailable"
	ServiceBindingGUIDLabel              = "korifi.cloudfoundry.org/service-binding-guid"
	ServiceCredentialBindingTypeLabel    = "korifi.cloudfoundry.org/service-credential-binding-type"
)

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebindings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebindings/finalizers,verbs=update
//+kubebuilder:rbac:groups=servicebinding.io,resources=servicebindings,verbs=get;list;create;update;patch;watch

func (r *CFServiceBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	cfServiceBinding := new(korifiv1alpha1.CFServiceBinding)
	err := r.Client.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, cfServiceBinding)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			r.log.Error(err, "unable to fetch CFServiceBinding", req.Name, req.Namespace)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	err = r.addFinalizer(ctx, cfServiceBinding)
	if err != nil {
		r.log.Error(err, "Error adding finalizer")
		return ctrl.Result{}, err
	}

	cfApp := new(korifiv1alpha1.CFApp)
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.AppRef.Name, Namespace: cfServiceBinding.Namespace}, cfApp)
	if err != nil {
		r.log.Error(err, "Error when fetching CFApp")

		// If CFApp is missing due to the use of background cascading delete, we expect CFServiceBinding delete to
		// proceed without further vcap services secret cleanup
		if apierrors.IsNotFound(err) && !cfServiceBinding.ObjectMeta.DeletionTimestamp.IsZero() {
			return r.finalizeCFServiceBinding(ctx, cfServiceBinding)
		}

		return ctrl.Result{}, err
	}

	originalCfServiceBinding := cfServiceBinding.DeepCopy()
	err = controllerutil.SetOwnerReference(cfApp, cfServiceBinding, r.scheme)
	if err != nil {
		r.log.Error(err, "Unable to set owner reference on CfServiceBinding")
		return ctrl.Result{}, err
	}

	err = r.Client.Patch(ctx, cfServiceBinding, client.MergeFrom(originalCfServiceBinding))
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error setting owner reference on the CFServiceBinding %s/%s", req.Namespace, cfServiceBinding.Name))
		return ctrl.Result{}, err
	}

	instance := new(korifiv1alpha1.CFServiceInstance)
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.Service.Name, Namespace: req.Namespace}, instance)
	if err != nil {
		// Unlike with CFApp cascading delete, CFServiceInstance delete cleans up CFServiceBindings itself as part of finalizing,
		// so we do not check for deletion timestamp before returning here.
		return r.handleGetError(ctx, err, cfServiceBinding, BindingSecretAvailableCondition, "ServiceInstanceNotFound", "Service instance")
	}

	secret := new(corev1.Secret)
	// Note: is there a reason to fetch the secret name from the service instance spec?
	err = r.Client.Get(ctx, types.NamespacedName{Name: instance.Spec.SecretName, Namespace: req.Namespace}, secret)
	if err != nil {
		return r.handleGetError(ctx, err, cfServiceBinding, BindingSecretAvailableCondition, "SecretNotFound", "Binding secret")
	}

	cfServiceBinding.Status.Binding.Name = instance.Spec.SecretName
	conditions.PatchStatus(ctx, r.Client, cfServiceBinding, metav1.Condition{
		Type:    BindingSecretAvailableCondition,
		Status:  metav1.ConditionTrue,
		Reason:  "SecretFound",
		Message: "",
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if cfApp.Status.VCAPServicesSecretName == "" {
		r.log.Info("Did not find VCAPServiceSecret name on status of CFApp", "CFServiceBinding", cfServiceBinding.Name)
		err = conditions.PatchStatus(ctx, r.Client, cfServiceBinding, metav1.Condition{
			Type:    VCAPServicesSecretAvailableCondition,
			Status:  metav1.ConditionFalse,
			Reason:  "SecretNotFound",
			Message: "VCAPServicesSecret name absent from status of CFApp",
		})
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}

	vcapServicesData, err := r.builder.BuildVCAPServicesEnvValue(ctx, cfApp)
	if err != nil {
		r.log.Error(err, "failed to build vcap services secret", "CFServiceBinding", cfServiceBinding)
	}

	vcapServicesSecret := new(corev1.Secret)
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfApp.Status.VCAPServicesSecretName, Namespace: req.Namespace}, vcapServicesSecret)
	if err != nil {
		return r.handleGetError(ctx, err, cfServiceBinding, VCAPServicesSecretAvailableCondition, "SecretNotFound", "Secret")
	}

	updatedVcapServicesSecret := vcapServicesSecret.DeepCopy()
	secretData := map[string][]byte{}
	secretData["VCAP_SERVICES"] = []byte(vcapServicesData)
	updatedVcapServicesSecret.Data = secretData
	err = r.Client.Patch(ctx, updatedVcapServicesSecret, client.MergeFrom(vcapServicesSecret))
	if err != nil {
		r.log.Error(err, "failed to patch vcap services secret", "CFServiceBinding", cfServiceBinding)
	}

	if !cfServiceBinding.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.finalizeCFServiceBinding(ctx, cfServiceBinding)
	}

	err = conditions.PatchStatus(ctx, r.Client, cfServiceBinding, metav1.Condition{
		Type:    VCAPServicesSecretAvailableCondition,
		Status:  metav1.ConditionTrue,
		Reason:  "SecretFound",
		Message: "",
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	actualSBServiceBinding := servicebindingv1beta1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cf-binding-%s", cfServiceBinding.Name),
			Namespace: cfServiceBinding.Namespace,
		},
	}

	desiredSBServiceBinding := generateDesiredServiceBinding(&actualSBServiceBinding, cfServiceBinding, cfApp, secret)

	_, err = controllerutil.CreateOrPatch(ctx, r.Client, &actualSBServiceBinding, sbServiceBindingMutateFn(&actualSBServiceBinding, desiredSBServiceBinding))
	if err != nil {
		r.log.Error(err, "Error calling Create on servicebinding.io ServiceBinding")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFServiceBindingReconciler) addFinalizer(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) error {
	if controllerutil.ContainsFinalizer(cfServiceBinding, CFServiceBindingFinalizerName) {
		return nil
	}

	originalCFServiceBinding := cfServiceBinding.DeepCopy()
	controllerutil.AddFinalizer(cfServiceBinding, CFServiceBindingFinalizerName)

	err := r.Client.Patch(ctx, cfServiceBinding, client.MergeFrom(originalCFServiceBinding))
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error adding finalizer to CFServiceBinding/%s", cfServiceBinding.Name))
		return err
	}

	r.log.Info(fmt.Sprintf("Finalizer added to CFServiceBinding/%s", cfServiceBinding.Name))
	return nil
}

func (r *CFServiceBindingReconciler) finalizeCFServiceBinding(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error) {
	r.log.Info(fmt.Sprintf("Reconciling deletion of CFServiceBinding/%s", cfServiceBinding.Name))

	if controllerutil.ContainsFinalizer(cfServiceBinding, CFServiceBindingFinalizerName) {
		originalCFServiceBinding := cfServiceBinding.DeepCopy()
		controllerutil.RemoveFinalizer(cfServiceBinding, CFServiceBindingFinalizerName)

		if err := r.Client.Patch(ctx, cfServiceBinding, client.MergeFrom(originalCFServiceBinding)); err != nil {
			r.log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *CFServiceBindingReconciler) handleGetError(ctx context.Context, err error, cfServiceBinding *korifiv1alpha1.CFServiceBinding, conditionType, notFoundReason, objectType string) (ctrl.Result, error) {
	if apierrors.IsNotFound(err) {
		cfServiceBinding.Status.Binding.Name = ""
		statusErr := conditions.PatchStatus(ctx, r.Client, cfServiceBinding, metav1.Condition{
			Type:    conditionType,
			Status:  metav1.ConditionFalse,
			Reason:  notFoundReason,
			Message: objectType + " does not exist",
		})
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}

	statusErr := conditions.PatchStatus(ctx, r.Client, cfServiceBinding, metav1.Condition{
		Type:    conditionType,
		Status:  metav1.ConditionFalse,
		Reason:  "UnknownError",
		Message: "Error occurred while fetching " + strings.ToLower(objectType) + ": " + err.Error(),
	})
	return ctrl.Result{}, statusErr
}

func sbServiceBindingMutateFn(actualSBServiceBinding, desiredSBServiceBinding *servicebindingv1beta1.ServiceBinding) controllerutil.MutateFn {
	return func() error {
		actualSBServiceBinding.ObjectMeta.Labels = desiredSBServiceBinding.ObjectMeta.Labels
		actualSBServiceBinding.ObjectMeta.OwnerReferences = desiredSBServiceBinding.ObjectMeta.OwnerReferences
		actualSBServiceBinding.Spec = desiredSBServiceBinding.Spec
		return nil
	}
}

func generateDesiredServiceBinding(actualServiceBinding *servicebindingv1beta1.ServiceBinding, cfServiceBinding *korifiv1alpha1.CFServiceBinding, cfApp *korifiv1alpha1.CFApp, secret *corev1.Secret) *servicebindingv1beta1.ServiceBinding {
	var desiredServiceBinding servicebindingv1beta1.ServiceBinding
	actualServiceBinding.DeepCopyInto(&desiredServiceBinding)
	desiredServiceBinding.ObjectMeta.Labels = map[string]string{
		ServiceBindingGUIDLabel:           cfServiceBinding.Name,
		korifiv1alpha1.CFAppGUIDLabelKey:  cfApp.Name,
		ServiceCredentialBindingTypeLabel: "app",
	}
	desiredServiceBinding.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "korifi.cloudfoundry.org/v1alpha1",
			Kind:       "CFServiceBinding",
			Name:       cfServiceBinding.Name,
			UID:        cfServiceBinding.UID,
		},
	}
	desiredServiceBinding.Spec = servicebindingv1beta1.ServiceBindingSpec{
		Name: secret.Name,
		Type: "user-provided",
		Workload: servicebindingv1beta1.ServiceBindingWorkloadReference{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					korifiv1alpha1.CFAppGUIDLabelKey: cfApp.Name,
				},
			},
		},
		Service: servicebindingv1beta1.ServiceBindingServiceReference{
			APIVersion: "korifi.cloudfoundry.org/v1alpha1",
			Kind:       "CFServiceBinding",
			Name:       cfServiceBinding.Name,
		},
	}
	secretType, ok := secret.Data["type"]
	if ok && len(secretType) > 0 {
		desiredServiceBinding.Spec.Type = string(secretType)
	}
	secretProvider, ok := secret.Data["provider"]
	if ok {
		desiredServiceBinding.Spec.Provider = string(secretProvider)
	}
	return &desiredServiceBinding
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFServiceBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceBinding{}).
		Complete(r)
}
