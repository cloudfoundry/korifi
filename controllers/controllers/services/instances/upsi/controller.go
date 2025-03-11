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

package upsi

import (
	"context"
	"encoding/json"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	k8sClient client.Client
	scheme    *runtime.Scheme
	log       logr.Logger
}

func NewReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.CFServiceInstance, *korifiv1alpha1.CFServiceInstance] {
	serviceInstanceReconciler := Reconciler{k8sClient: client, scheme: scheme, log: log}
	return k8s.NewPatchingReconciler(log, client, &serviceInstanceReconciler)
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceInstance{}).
		Named("user-provided-cfserviceinstance").
		WithEventFilter(predicate.NewPredicateFuncs(r.isUPSI)).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.secretToServiceInstance),
		)
}

func (r *Reconciler) isUPSI(object client.Object) bool {
	serviceInstance, ok := object.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		return true
	}

	return serviceInstance.Spec.Type == korifiv1alpha1.UserProvidedType
}

func (r *Reconciler) secretToServiceInstance(ctx context.Context, o client.Object) []reconcile.Request {
	serviceInstances := korifiv1alpha1.CFServiceInstanceList{}
	if err := r.k8sClient.List(ctx, &serviceInstances,
		client.InNamespace(o.GetNamespace()),
		client.MatchingFields{
			shared.IndexServiceInstanceCredentialsSecretName: o.GetName(),
		}); err != nil {
		return []reconcile.Request{}
	}

	requests := []reconcile.Request{}
	for _, si := range serviceInstances.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      si.Name,
				Namespace: si.Namespace,
			},
		})
	}

	return requests
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceinstances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceinstances/finalizers,verbs=update

func (r *Reconciler) ReconcileResource(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	cfServiceInstance.Status.ObservedGeneration = cfServiceInstance.Generation
	log.V(1).Info("set observed generation", "generation", cfServiceInstance.Status.ObservedGeneration)

	if !cfServiceInstance.GetDeletionTimestamp().IsZero() {
		controllerutil.RemoveFinalizer(cfServiceInstance, korifiv1alpha1.CFServiceInstanceFinalizerName)
		log.V(1).Info("finalizer removed")

		return ctrl.Result{}, nil
	}

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceInstance.Namespace,
			Name:      cfServiceInstance.Spec.SecretName,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)
	if err != nil {
		notReadyErr := k8s.NewNotReadyError().WithCause(err).WithReason("CredentialsSecretNotAvailable")
		if apierrors.IsNotFound(err) {
			notReadyErr = notReadyErr.WithRequeueAfter(2 * time.Second)
		}
		return ctrl.Result{}, notReadyErr
	}

	if err = r.validateCredentials(credentialsSecret); err != nil {
		cfServiceInstance.Status.LastOperation = korifiv1alpha1.LastOperation{
			Type:  "create",
			State: "failed",
		}
		return ctrl.Result{}, k8s.NewNotReadyError().WithCause(err).WithReason("SecretInvalid").WithNoRequeue()
	}

	log.V(1).Info("credentials secret", "name", credentialsSecret.Name, "version", credentialsSecret.ResourceVersion)
	cfServiceInstance.Status.Credentials = corev1.LocalObjectReference{Name: credentialsSecret.Name}

	cfServiceInstance.Status.LastOperation = reconcileLastOperation(cfServiceInstance, credentialsSecret)

	cfServiceInstance.Status.CredentialsObservedVersion = credentialsSecret.ResourceVersion

	return ctrl.Result{}, nil
}

func (r *Reconciler) validateCredentials(credentialsSecret *corev1.Secret) error {
	return errors.Wrapf(
		json.Unmarshal(credentialsSecret.Data[tools.CredentialsSecretKey], &map[string]any{}),
		"invalid credentials secret %q",
		credentialsSecret.Name,
	)
}

func reconcileLastOperation(cfServiceInstance *korifiv1alpha1.CFServiceInstance, credentialsSecret *corev1.Secret) korifiv1alpha1.LastOperation {
	if cfServiceInstance.Status.CredentialsObservedVersion == "" {
		return korifiv1alpha1.LastOperation{
			Type:  "create",
			State: "succeeded",
		}
	}
	if cfServiceInstance.Status.CredentialsObservedVersion != credentialsSecret.ResourceVersion && cfServiceInstance.Status.CredentialsObservedVersion != "" {
		return korifiv1alpha1.LastOperation{
			Type:  "update",
			State: "succeeded",
		}
	}
	return cfServiceInstance.Status.LastOperation
}
