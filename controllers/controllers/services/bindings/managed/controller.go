package managed

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/controllers/controllers/services/credentials"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

type ManagedBindingsReconciler struct {
	k8sClient           client.Client
	osbapiClientFactory osbapi.BrokerClientFactory
	scheme              *runtime.Scheme
	assets              *osbapi.Assets
}

func NewReconciler(k8sClient client.Client, brokerClientFactory osbapi.BrokerClientFactory, rootNamespace string, scheme *runtime.Scheme) *ManagedBindingsReconciler {
	return &ManagedBindingsReconciler{
		k8sClient:           k8sClient,
		osbapiClientFactory: brokerClientFactory,
		scheme:              scheme,
		assets:              osbapi.NewAssets(k8sClient, rootNamespace),
	}
}

func (r *ManagedBindingsReconciler) ReconcileResource(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("reconcile-managed-service-binding")

	if !cfServiceBinding.GetDeletionTimestamp().IsZero() {
		return r.finalize(ctx, cfServiceBinding)
	}

	assets, err := r.assets.GetServiceBindingAssets(ctx, cfServiceBinding)
	if err != nil {
		log.Error(err, "failed to get service binding assets")
		return ctrl.Result{}, err
	}

	osbapiClient, err := r.osbapiClientFactory.CreateClient(ctx, assets.ServiceBroker)
	if err != nil {
		log.Error(err, "failed to create broker client", "broker", assets.ServiceBroker.Name)
		return ctrl.Result{}, err
	}

	if isReconciled(cfServiceBinding) {
		return ctrl.Result{}, nil
	}

	if isFailed(cfServiceBinding) {
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("BindingFailed").WithNoRequeue()
	}

	cfServiceBinding.Labels = tools.SetMapValue(cfServiceBinding.Labels, korifiv1alpha1.PlanGUIDLabelKey, assets.ServicePlan.Name)

	bindResponse, err := r.bind(ctx, cfServiceBinding, assets, osbapiClient)
	if err != nil {
		return ctrl.Result{}, err
	}

	if bindResponse.IsAsync {
		var lastOpResponse osbapi.LastOperationResponse
		lastOpResponse, err = r.pollLastOperation(ctx, cfServiceBinding, assets, osbapiClient, bindResponse.Operation)
		if err != nil {
			return ctrl.Result{}, err
		}

		return r.processBindOperation(cfServiceBinding, lastOpResponse)
	}

	envSecret, err := r.createEnvSecret(ctx, cfServiceBinding, bindResponse.Credentials)
	if err != nil {
		return ctrl.Result{}, err
	}

	cfServiceBinding.Status.EnvSecretRef.Name = envSecret.Name

	if cfServiceBinding.Spec.Type == korifiv1alpha1.CFServiceBindingTypeKey {
		return ctrl.Result{}, nil
	}

	mountSecret, err := r.createMountSecret(ctx, cfServiceBinding, bindResponse.Credentials)
	if err != nil {
		return ctrl.Result{}, err
	}

	cfServiceBinding.Status.MountSecretRef.Name = mountSecret.Name

	return ctrl.Result{}, nil
}

func (r *ManagedBindingsReconciler) bind(
	ctx context.Context,
	cfServiceBinding *korifiv1alpha1.CFServiceBinding,
	assets osbapi.ServiceBindingAssets,
	osbapiClient osbapi.BrokerClient,
) (osbapi.BindResponse, error) {
	log := logr.FromContextOrDiscard(ctx)

	parameters, err := r.getParameters(ctx, cfServiceBinding)
	if err != nil {
		return osbapi.BindResponse{}, k8s.NewNotReadyError().WithReason("InvalidParameters")
	}

	bindResponse, err := osbapiClient.Bind(ctx, osbapi.BindPayload{
		BindingID:  cfServiceBinding.Name,
		InstanceID: assets.ServiceInstance.Name,
		BindRequest: osbapi.BindRequest{
			ServiceId: assets.ServiceOffering.Spec.BrokerCatalog.ID,
			PlanID:    assets.ServicePlan.Spec.BrokerCatalog.ID,
			AppGUID:   cfServiceBinding.Spec.AppRef.Name,
			BindResource: osbapi.BindResource{
				AppGUID: cfServiceBinding.Spec.AppRef.Name,
			},
			Parameters: parameters,
		},
	})
	if err != nil {
		log.Error(err, "failed to bind")

		if osbapi.IsUnrecoveralbeError(err) {
			meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
				Type:               korifiv1alpha1.BindingFailedCondition,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: cfServiceBinding.Generation,
				LastTransitionTime: metav1.NewTime(time.Now()),
				Reason:             "BindingFailed",
				Message:            err.Error(),
			})
			return osbapi.BindResponse{}, k8s.NewNotReadyError().WithReason("BindingFailed")
		}

		return osbapi.BindResponse{}, err
	}

	return bindResponse, nil
}

func (r *ManagedBindingsReconciler) getParameters(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (map[string]any, error) {
	if cfServiceBinding.Spec.Parameters.Name == "" {
		return nil, nil
	}

	paramsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceBinding.Namespace,
			Name:      cfServiceBinding.Spec.Parameters.Name,
		},
	}

	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(paramsSecret), paramsSecret)
	if err != nil {
		return nil, err
	}

	return tools.FromParametersSecretData(paramsSecret.Data)
}

func (r *ManagedBindingsReconciler) processBindOperation(
	cfServiceBinding *korifiv1alpha1.CFServiceBinding,
	lastOperation osbapi.LastOperationResponse,
) (ctrl.Result, error) {
	if lastOperation.State == "succeeded" {
		return ctrl.Result{Requeue: true}, nil
	}

	if lastOperation.State == "failed" {
		meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.BindingFailedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cfServiceBinding.Generation,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "BindingFailed",
			Message:            lastOperation.Description,
		})
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("BindingFailed").WithMessage(lastOperation.Description)
	}

	return ctrl.Result{}, k8s.NewNotReadyError().WithReason("BindingInProgress").WithRequeueAfter(time.Second)
}

func (r *ManagedBindingsReconciler) createEnvSecret(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding, creds map[string]any) (*corev1.Secret, error) {
	log := logr.FromContextOrDiscard(ctx)

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceBinding.Name,
			Namespace: cfServiceBinding.Namespace,
		},
	}
	_, err := controllerutil.CreateOrPatch(ctx, r.k8sClient, credentialsSecret, func() error {
		credentialsSecretData, err := tools.ToCredentialsSecretData(creds)
		if err != nil {
			return err
		}
		credentialsSecret.Data = credentialsSecretData
		return controllerutil.SetControllerReference(cfServiceBinding, credentialsSecret, r.scheme)
	})
	if err != nil {
		log.Error(err, "failed to create credentials secret")
		return nil, err
	}

	return credentialsSecret, nil
}

func (r *ManagedBindingsReconciler) createMountSecret(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding, creds map[string]any) (*corev1.Secret, error) {
	log := logr.FromContextOrDiscard(ctx)

	mountSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceBinding.Name + "-sbio",
			Namespace: cfServiceBinding.Namespace,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, r.k8sClient, mountSecret, func() error {
		mountSecret.Type = corev1.SecretType(credentials.ServiceBindingSecretTypePrefix + korifiv1alpha1.ManagedType)
		var err error
		mountSecret.Data, err = credentials.GetServiceBindingIOSecretData(creds)
		if err != nil {
			return err
		}

		return controllerutil.SetControllerReference(cfServiceBinding, mountSecret, r.scheme)
	})
	if err != nil {
		log.Error(err, "failed to create binding secret")
		return nil, err
	}

	return mountSecret, nil
}

func (r *ManagedBindingsReconciler) finalize(
	ctx context.Context,
	serviceBinding *korifiv1alpha1.CFServiceBinding,
) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("finalize-managed-service-binding")

	if !controllerutil.ContainsFinalizer(serviceBinding, korifiv1alpha1.CFServiceBindingFinalizerName) {
		return ctrl.Result{}, nil
	}

	err := r.finalizeCFServiceBinding(ctx, serviceBinding)
	if err != nil {
		return ctrl.Result{}, err
	}

	if controllerutil.RemoveFinalizer(serviceBinding, korifiv1alpha1.CFServiceBindingFinalizerName) {
		log.V(1).Info("finalizer removed")
	}
	return ctrl.Result{}, nil
}

func (r *ManagedBindingsReconciler) finalizeCFServiceBinding(
	ctx context.Context,
	serviceBinding *korifiv1alpha1.CFServiceBinding,
) error {
	serviceInstance := &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: serviceBinding.Namespace,
			Name:      serviceBinding.Spec.Service.Name,
		},
	}

	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceInstance), serviceInstance)
	if err != nil {
		return fmt.Errorf("failed to get service instance: %w", err)
	}

	if serviceInstance.Annotations[korifiv1alpha1.DeprovisionWithoutBrokerAnnotation] == "true" {
		return nil
	}

	assets, err := r.assets.GetServiceBindingAssets(ctx, serviceBinding)
	if err != nil {
		return fmt.Errorf("failed to get service binding assets: %w", err)
	}

	osbapiClient, err := r.osbapiClientFactory.CreateClient(ctx, assets.ServiceBroker)
	if err != nil {
		return fmt.Errorf("failed to client for broker %q: %w", assets.ServiceBroker.Name, err)
	}

	unbindResponse, err := r.deleteServiceBinding(ctx, serviceBinding, assets, osbapiClient)
	if err != nil {
		return err
	}
	if unbindResponse.IsAsync {
		lastOpresponse, err := r.pollLastOperation(ctx, serviceBinding, assets, osbapiClient, unbindResponse.Operation)
		if err != nil {
			return err
		}

		return r.processUnbindLastOperation(serviceBinding, lastOpresponse)
	}

	return nil
}

func (r *ManagedBindingsReconciler) pollLastOperation(
	ctx context.Context,
	serviceBinding *korifiv1alpha1.CFServiceBinding,
	assets osbapi.ServiceBindingAssets,
	osbapiClient osbapi.BrokerClient,
	operationID string,
) (osbapi.LastOperationResponse, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("poll-operation")

	lastOpResponse, err := osbapiClient.GetServiceBindingLastOperation(ctx, osbapi.GetBindingLastOperationRequest{
		InstanceID: serviceBinding.Spec.Service.Name,
		BindingID:  serviceBinding.Name,
		GetLastOperationRequestParameters: osbapi.GetLastOperationRequestParameters{
			ServiceId: assets.ServiceOffering.Spec.BrokerCatalog.ID,
			PlanID:    assets.ServicePlan.Spec.BrokerCatalog.ID,
			Operation: operationID,
		},
	})
	if err != nil {
		log.Error(err, "getting service binding last operation failed")
		return osbapi.LastOperationResponse{}, k8s.NewNotReadyError().WithCause(err).WithReason("GetLastOperationFailed")

	}
	return lastOpResponse, nil
}

func (r *ManagedBindingsReconciler) processUnbindLastOperation(
	serviceBinding *korifiv1alpha1.CFServiceBinding,
	lastOpResponse osbapi.LastOperationResponse,
) error {
	if lastOpResponse.State == "succeeded" {
		return nil
	}

	if lastOpResponse.State == "failed" {
		meta.SetStatusCondition(&serviceBinding.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.UnbindingFailedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: serviceBinding.Generation,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "UnbindingFailed",
			Message:            lastOpResponse.Description,
		})
		return k8s.NewNotReadyError().WithReason("UnbindFailed")

	}
	return k8s.NewNotReadyError().WithReason("UnbindingInProgress").WithRequeue()
}

func (r *ManagedBindingsReconciler) deleteServiceBinding(
	ctx context.Context,
	serviceBinding *korifiv1alpha1.CFServiceBinding,
	assets osbapi.ServiceBindingAssets,
	osbapiClient osbapi.BrokerClient,
) (osbapi.UnbindResponse, error) {
	unbindResponse, err := osbapiClient.Unbind(ctx, osbapi.UnbindPayload{
		InstanceID: serviceBinding.Spec.Service.Name,
		BindingID:  serviceBinding.Name,
		UnbindRequestParameters: osbapi.UnbindRequestParameters{
			ServiceId: assets.ServiceOffering.Spec.BrokerCatalog.ID,
			PlanID:    assets.ServicePlan.Spec.BrokerCatalog.ID,
		},
	})
	if osbapi.IgnoreGone(err) != nil {
		if osbapi.IsUnrecoveralbeError(err) {
			meta.SetStatusCondition(&serviceBinding.Status.Conditions, metav1.Condition{
				Type:               korifiv1alpha1.UnbindingFailedCondition,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: serviceBinding.Generation,
				LastTransitionTime: metav1.NewTime(time.Now()),
				Reason:             "UnbindingFailed",
				Message:            err.Error(),
			})
			return osbapi.UnbindResponse{}, k8s.NewNotReadyError().WithReason("UnbindingFailed")
		}

		return osbapi.UnbindResponse{}, fmt.Errorf("failed to unbind: %w", err)
	}

	return unbindResponse, nil
}

func isFailed(binding *korifiv1alpha1.CFServiceBinding) bool {
	return meta.IsStatusConditionTrue(binding.Status.Conditions, korifiv1alpha1.BindingFailedCondition)
}

func isReconciled(binding *korifiv1alpha1.CFServiceBinding) bool {
	return binding.Status.EnvSecretRef.Name != "" && binding.Status.MountSecretRef.Name != ""
}
