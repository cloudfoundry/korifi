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

package instances

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/bindings"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
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
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFServiceInstance, *korifiv1alpha1.CFServiceInstance](log, client, &serviceInstanceReconciler)
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceInstance{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.secretToServiceInstance),
		)
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

	var err error
	readyConditionBuilder := k8s.NewReadyConditionBuilder(cfServiceInstance)
	defer func() {
		meta.SetStatusCondition(&cfServiceInstance.Status.Conditions, readyConditionBuilder.WithError(err).Build())
	}()

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceInstance.Namespace,
			Name:      cfServiceInstance.Spec.SecretName,
		},
	}
	err = r.k8sClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)
	if err != nil {
		readyConditionBuilder.WithReason("CredentialsSecretNotAvailable")
		if apierrors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	credentialsSecret, err = r.reconcileCredentials(ctx, credentialsSecret, cfServiceInstance)
	if err != nil {
		readyConditionBuilder.WithReason("FailedReconcilingCredentialsSecret")
		return ctrl.Result{}, err
	}

	if err = r.validateCredentials(ctx, credentialsSecret); err != nil {
		readyConditionBuilder.WithReason("SecretInvalid")
		return ctrl.Result{}, err
	}

	log.V(1).Info("credentials secret", "name", credentialsSecret.Name, "version", credentialsSecret.ResourceVersion)
	cfServiceInstance.Status.Credentials = corev1.LocalObjectReference{Name: credentialsSecret.Name}
	cfServiceInstance.Status.CredentialsObservedVersion = credentialsSecret.ResourceVersion

	readyConditionBuilder.Ready()
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileCredentials(ctx context.Context, credentialsSecret *corev1.Secret, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (*corev1.Secret, error) {
	if !strings.HasPrefix(string(credentialsSecret.Type), bindings.ServiceBindingSecretTypePrefix) {
		return credentialsSecret, nil
	}

	log := logr.FromContextOrDiscard(ctx)

	log.Info("migrating legacy secret", "legacy-secret-name", credentialsSecret.Name)
	migratedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceInstance.Name + "-migrated",
			Namespace: cfServiceInstance.Namespace,
		},
	}
	_, err := controllerutil.CreateOrPatch(ctx, r.k8sClient, migratedSecret, func() error {
		migratedSecret.Type = corev1.SecretTypeOpaque
		data := map[string]any{}
		for k, v := range credentialsSecret.Data {
			data[k] = string(v)
		}

		dataBytes, err := json.Marshal(data)
		if err != nil {
			log.Error(err, "failed to marshal legacy credentials secret data")
			return err
		}

		migratedSecret.Data = map[string][]byte{
			korifiv1alpha1.CredentialsSecretKey: dataBytes,
		}
		return controllerutil.SetOwnerReference(cfServiceInstance, migratedSecret, r.scheme)
	})
	if err != nil {
		log.Error(err, "failed to create migrated credentials secret")
		return nil, err
	}

	return migratedSecret, nil
}

func (r *Reconciler) validateCredentials(ctx context.Context, credentialsSecret *corev1.Secret) error {
	return errors.Wrapf(
		json.Unmarshal(credentialsSecret.Data[korifiv1alpha1.CredentialsSecretKey], &map[string]any{}),
		"invalid credentials secret %q",
		credentialsSecret.Name,
	)
}
