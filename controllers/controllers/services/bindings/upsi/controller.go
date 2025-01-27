package upsi

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/bindings/sbio"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/credentials"
	servicebindingv1beta1 "github.com/servicebinding/runtime/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

type UPSIBindingReconciler struct {
	k8sClient client.Client
	scheme    *runtime.Scheme
}

func NewReconciler(k8sClient client.Client, scheme *runtime.Scheme) *UPSIBindingReconciler {
	return &UPSIBindingReconciler{
		k8sClient: k8sClient,
		scheme:    scheme,
	}
}

func (r *UPSIBindingReconciler) ReconcileResource(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	if !cfServiceBinding.GetDeletionTimestamp().IsZero() {
		if controllerutil.RemoveFinalizer(cfServiceBinding, korifiv1alpha1.CFServiceBindingFinalizerName) {
			log.V(1).Info("finalizer removed")
		}

		return ctrl.Result{}, nil
	}

	cfServiceInstance := new(korifiv1alpha1.CFServiceInstance)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.Service.Name, Namespace: cfServiceBinding.Namespace}, cfServiceInstance)
	if err != nil {
		log.Info("service instance not found", "service-instance", cfServiceBinding.Spec.Service.Name, "error", err)
		return ctrl.Result{}, err
	}

	if cfServiceInstance.Status.Credentials.Name == "" {
		return ctrl.Result{}, k8s.NewNotReadyError().
			WithReason("CredentialsSecretNotAvailable").
			WithMessage("Service instance credentials not available yet").
			WithRequeueAfter(time.Second)
	}

	bindingSecret, err := r.reconcileCredentials(ctx, cfServiceInstance, cfServiceBinding)
	if err != nil {
		if k8serrors.IsInvalid(err) {
			err = r.k8sClient.Delete(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfServiceBinding.Name,
					Namespace: cfServiceBinding.Namespace,
				},
			})
			return ctrl.Result{}, errors.Wrap(err, "failed to delete outdated binding secret")
		}

		log.Error(err, "failed to reconcile credentials secret")
		return ctrl.Result{}, err
	}

	sbServiceBinding, err := r.reconcileSBServiceBinding(ctx, cfServiceBinding, bindingSecret)
	if err != nil {
		log.Info("error creating/updating servicebinding.io servicebinding", "reason", err)
		return ctrl.Result{}, err
	}

	if !sbio.IsSbServiceBindingReady(sbServiceBinding) {
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("ServiceBindingNotReady")
	}

	return ctrl.Result{}, nil
}

func (r *UPSIBindingReconciler) reconcileCredentials(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (*corev1.Secret, error) {
	cfServiceBinding.Status.Credentials.Name = cfServiceInstance.Status.Credentials.Name

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceInstance.Namespace,
			Name:      cfServiceInstance.Status.Credentials.Name,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to get service instance credentials secret %q: %w", cfServiceInstance.Status.Credentials.Name, err)
	}

	bindingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceBinding.Name,
			Namespace: cfServiceBinding.Namespace,
		},
	}

	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, bindingSecret, func() error {
		bindingSecret.Type, err = credentials.GetBindingSecretType(credentialsSecret)
		if err != nil {
			return err
		}
		bindingSecret.Data, err = credentials.GetUserProvidedServiceBindingIOSecretData(credentialsSecret)
		if err != nil {
			return err
		}

		return controllerutil.SetControllerReference(cfServiceBinding, bindingSecret, r.scheme)
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create binding secret")
	}

	cfServiceBinding.Status.Binding.Name = bindingSecret.Name

	return bindingSecret, nil
}

func (r *UPSIBindingReconciler) reconcileSBServiceBinding(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding, bindingSecret *corev1.Secret) (*servicebindingv1beta1.ServiceBinding, error) {
	sbServiceBinding := sbio.ToSBServiceBinding(cfServiceBinding, korifiv1alpha1.UserProvidedType)

	_, err := controllerutil.CreateOrPatch(ctx, r.k8sClient, sbServiceBinding, func() error {
		secretType, hasType := bindingSecret.Data["type"]
		if hasType && len(secretType) > 0 {
			sbServiceBinding.Spec.Type = string(secretType)
		}

		secretProvider, hasProvider := bindingSecret.Data["provider"]
		if hasProvider {
			sbServiceBinding.Spec.Provider = string(secretProvider)
		}
		return controllerutil.SetControllerReference(cfServiceBinding, sbServiceBinding, r.scheme)
	})
	if err != nil {
		return nil, err
	}

	return sbServiceBinding, nil
}
