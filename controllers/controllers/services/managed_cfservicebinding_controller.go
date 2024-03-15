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
	"encoding/json"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
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
	BindRequestedCondition   = "BindRequested"
	UnbindRequestedCondition = "UnbindRequested"
)

type ManagedCFServiceBindingReconciler struct {
	log          logr.Logger
	k8sClient    client.Client
	scheme       *runtime.Scheme
	brokerClient BrokerClient
}

func NewManagedCFServiceBindingReconciler(
	log logr.Logger,
	k8sClient client.Client,
	scheme *runtime.Scheme,
	brokerClient BrokerClient,
) *k8s.PatchingReconciler[korifiv1alpha1.CFServiceBinding, *korifiv1alpha1.CFServiceBinding] {
	cfBindingReconciler := &ManagedCFServiceBindingReconciler{
		log:          log,
		k8sClient:    k8sClient,
		scheme:       scheme,
		brokerClient: brokerClient,
	}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFServiceBinding, *korifiv1alpha1.CFServiceBinding](log, k8sClient, cfBindingReconciler)
}

func (r *ManagedCFServiceBindingReconciler) ReconcileResource(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	managedBinding, err := r.isManaged(ctx, cfServiceBinding)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !managedBinding {
		log.V(1).Info("skipping binding for managed service instance", "name", cfServiceBinding.Name)
		return ctrl.Result{}, nil
	}

	controllerutil.AddFinalizer(cfServiceBinding, korifiv1alpha1.ManagedCFServiceBindingFinalizerName)
	if !cfServiceBinding.GetDeletionTimestamp().IsZero() {
		return r.finalizeCFServiceBinding(ctx, cfServiceBinding)
	}

	cfServiceBinding.Status.ObservedGeneration = cfServiceBinding.Generation
	log.V(1).Info("set observed generation", "generation", cfServiceBinding.Status.ObservedGeneration)

	if meta.IsStatusConditionTrue(cfServiceBinding.Status.Conditions, ReadyCondition) ||
		meta.IsStatusConditionTrue(cfServiceBinding.Status.Conditions, FailedCondition) {
		return ctrl.Result{}, nil
	}

	if !meta.IsStatusConditionTrue(cfServiceBinding.Status.Conditions, BindRequestedCondition) {
		err = r.brokerClient.BindService(ctx, cfServiceBinding)
		if err != nil {
			log.Error(err, "bind request failed")
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
			Type:               BindRequestedCondition,
			Status:             metav1.ConditionTrue,
			Reason:             "BindRequested",
			Message:            "Binding to service has been requested from broker",
			ObservedGeneration: cfServiceBinding.Generation,
		})

		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	lastOp, err := r.brokerClient.GetServiceBindingLastOperation(ctx, cfServiceBinding)
	if err != nil {
		log.Error(err, "get state failed")
		return ctrl.Result{}, err
	}
	if !lastOp.Exists {
		return ctrl.Result{}, fmt.Errorf("last operation for service binding %q not found", cfServiceBinding.Name)
	}

	if lastOp.State == "failed" {
		meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
			Type:               FailedCondition,
			Status:             metav1.ConditionTrue,
			Reason:             "Failed",
			Message:            lastOp.Description,
			ObservedGeneration: cfServiceBinding.Generation,
		})

		return ctrl.Result{}, nil
	}

	if lastOp.State != "succeeded" {
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	binding, err := r.brokerClient.GetServiceBinding(ctx, cfServiceBinding)
	if err != nil {
		return ctrl.Result{}, err
	}

	bindingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceBinding.Namespace,
			Name:      cfServiceBinding.Name + "-binding",
		},
	}

	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, bindingSecret, func() error {
		bindingSecret.Data = map[string][]byte{}
		for k, v := range binding.Credentials {
			valueString, isString := v.(string)
			if isString {
				bindingSecret.Data[k] = []byte(valueString)
				continue
			}
			valueBytes, marshalErr := json.Marshal(v)
			if marshalErr != nil {
				return fmt.Errorf("failed to marshal value: %w", marshalErr)
			}
			bindingSecret.Data[k] = valueBytes
		}

		return controllerutil.SetControllerReference(cfServiceBinding, bindingSecret, r.scheme)
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	cfServiceBinding.Status.Binding.Name = bindingSecret.Name

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceBinding.Namespace,
			Name:      cfServiceBinding.Name + "-credentials",
		},
	}

	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, credentialsSecret, func() error {
		credentialsBytes, err := json.Marshal(binding.Credentials)
		if err != nil {
			return fmt.Errorf("failed to marshal value: %w", err)
		}
		credentialsSecret.Data = map[string][]byte{
			korifiv1alpha1.CredentialsSecretKey: credentialsBytes,
		}

		return controllerutil.SetControllerReference(cfServiceBinding, credentialsSecret, r.scheme)
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	cfServiceBinding.Status.Credentials.Name = credentialsSecret.Name

	cfApp := new(korifiv1alpha1.CFApp)
	err = r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.AppRef.Name, Namespace: cfServiceBinding.Namespace}, cfApp)
	if err != nil {
		log.Info("error when fetching CFApp", "reason", err)
		return ctrl.Result{}, err
	}

	if cfApp.Status.VCAPServicesSecretName == "" {
		log.V(1).Info("did not find VCAPServiceSecret name on status of CFApp", "CFServiceBinding", cfServiceBinding.Name)
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
		Type:               VCAPServicesSecretAvailableCondition,
		Status:             metav1.ConditionTrue,
		Reason:             "SecretFound",
		Message:            "",
		ObservedGeneration: cfServiceBinding.Generation,
	})

	meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
		Type:               ReadyCondition,
		Status:             metav1.ConditionTrue,
		Reason:             "Ready",
		Message:            lastOp.Description,
		ObservedGeneration: cfServiceBinding.Generation,
	})

	return ctrl.Result{}, nil
}

func (r *ManagedCFServiceBindingReconciler) finalizeCFServiceBinding(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("finalizeCFServiceBinding")

	if !controllerutil.ContainsFinalizer(cfServiceBinding, korifiv1alpha1.ManagedCFServiceBindingFinalizerName) {
		return ctrl.Result{}, nil
	}

	if !meta.IsStatusConditionTrue(cfServiceBinding.Status.Conditions, UnbindRequestedCondition) {
		err := r.brokerClient.UnbindService(ctx, cfServiceBinding)
		if err != nil {
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
			Type:               UnbindRequestedCondition,
			Status:             metav1.ConditionTrue,
			Reason:             "UnbindRequested",
			ObservedGeneration: cfServiceBinding.Generation,
		})

		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	lastOp, err := r.brokerClient.GetServiceBindingLastOperation(ctx, cfServiceBinding)
	if !lastOp.Exists {
		if controllerutil.RemoveFinalizer(cfServiceBinding, korifiv1alpha1.ManagedCFServiceBindingFinalizerName) {
			log.V(1).Info("finalizer removed")
		}
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *ManagedCFServiceBindingReconciler) isManaged(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (bool, error) {
	cfServiceInstance, err := r.getServiceInstance(ctx, cfServiceBinding)
	if err != nil {
		return false, err
	}

	return cfServiceInstance.Spec.Type == korifiv1alpha1.ManagedType, nil
}

func (r *ManagedCFServiceBindingReconciler) getServiceInstance(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (*korifiv1alpha1.CFServiceInstance, error) {
	cfServiceInstance := &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceBinding.Spec.Service.Name,
			Namespace: cfServiceBinding.Namespace,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)
	if err != nil {
		return nil, err
	}

	return cfServiceInstance, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedCFServiceBindingReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceBinding{})
}
