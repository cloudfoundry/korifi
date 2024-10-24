package managed

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"code.cloudfoundry.org/korifi/controllers/controllers/services/credentials"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

type ManagedCredentialsReconciler struct {
	k8sClient           client.Client
	osbapiClientFactory osbapi.BrokerClientFactory
	scheme              *runtime.Scheme
	assets              *osbapi.Assets
}

func NewReconciler(k8sClient client.Client, brokerClientFactory osbapi.BrokerClientFactory, rootNamespace string, scheme *runtime.Scheme) *ManagedCredentialsReconciler {
	return &ManagedCredentialsReconciler{
		k8sClient:           k8sClient,
		osbapiClientFactory: brokerClientFactory,
		scheme:              scheme,
		assets:              osbapi.NewAssets(k8sClient, rootNamespace),
	}
}

func (r *ManagedCredentialsReconciler) ReconcileResource(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error) {
	if isReconciled(cfServiceBinding) {
		return ctrl.Result{}, nil
	}

	if isFailed(cfServiceBinding) {
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("BindingFailed").WithNoRequeue()
	}

	credentials, err := r.bind(ctx, cfServiceBinding)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.reconcileCredentials(ctx, cfServiceBinding, credentials)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ManagedCredentialsReconciler) bind(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (map[string]any, error) {
	log := logr.FromContextOrDiscard(ctx)

	cfServiceInstance := &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceBinding.Namespace,
			Name:      cfServiceBinding.Spec.Service.Name,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)
	if err != nil {
		log.Error(err, "failed to get service instance", "service-instance", cfServiceInstance.Name)
		return nil, err
	}

	servicePlan, err := r.assets.GetServicePlan(ctx, cfServiceInstance.Spec.PlanGUID)
	if err != nil {
		log.Error(err, "failed to get service plan", "service-plan", cfServiceInstance.Spec.PlanGUID)
		return nil, err
	}

	serviceBroker, err := r.assets.GetServiceBroker(ctx, servicePlan.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel])
	if err != nil {
		log.Error(err, "failed to get service broker", "service-broker", servicePlan.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel])
		return nil, err
	}

	serviceOffering, err := r.assets.GetServiceOffering(ctx, servicePlan.Labels[korifiv1alpha1.RelServiceOfferingGUIDLabel])
	if err != nil {
		log.Error(err, "failed to get service offering", "service-offering", servicePlan.Labels[korifiv1alpha1.RelServiceOfferingGUIDLabel])
		return nil, err
	}

	osbapiClient, err := r.osbapiClientFactory.CreateClient(ctx, serviceBroker)
	if err != nil {
		log.Error(err, "failed to create broker client", "broker", serviceBroker.Name)
		return nil, err
	}

	if !isBindRequested(cfServiceBinding) {
		return r.requestBinding(ctx, cfServiceBinding, cfServiceInstance, serviceOffering, servicePlan, osbapiClient)
	}

	return r.pollBinding(ctx, cfServiceBinding, serviceOffering, servicePlan, osbapiClient)
}

func (r *ManagedCredentialsReconciler) requestBinding(
	ctx context.Context,
	cfServiceBinding *korifiv1alpha1.CFServiceBinding,
	cfServiceInstance *korifiv1alpha1.CFServiceInstance,
	serviceOffering *korifiv1alpha1.CFServiceOffering,
	servicePlan *korifiv1alpha1.CFServicePlan,
	osbapiClient osbapi.BrokerClient,
) (map[string]any, error) {
	log := logr.FromContextOrDiscard(ctx)

	bindResponse, err := osbapiClient.Bind(ctx, osbapi.BindPayload{
		BindingID:  cfServiceBinding.Name,
		InstanceID: cfServiceInstance.Name,
		BindRequest: osbapi.BindRequest{
			ServiceId: serviceOffering.Spec.BrokerCatalog.ID,
			PlanID:    servicePlan.Spec.BrokerCatalog.ID,
			AppGUID:   cfServiceBinding.Spec.AppRef.Name,
			BindResource: osbapi.BindResource{
				AppGUID: cfServiceBinding.Spec.AppRef.Name,
			},
		},
	})
	if err != nil {
		log.Error(err, "failed to bind service")

		meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.BindingFailedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cfServiceBinding.Generation,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "BindingFailed",
			Message:            err.Error(),
		})

		return nil, k8s.NewNotReadyError().WithReason("BindingFailed")
	}

	cfServiceBinding.Status.BindingOperation = bindResponse.Operation
	meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
		Type:               korifiv1alpha1.BindingRequestedCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cfServiceBinding.Generation,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "BindingRequested",
	})

	if bindResponse.Complete {
		return bindResponse.Credentials, nil
	}

	return nil, k8s.NewNotReadyError().WithReason("BindingInProgress").WithRequeue()
}

func (r *ManagedCredentialsReconciler) pollBinding(
	ctx context.Context,
	cfServiceBinding *korifiv1alpha1.CFServiceBinding,
	serviceOffering *korifiv1alpha1.CFServiceOffering,
	servicePlan *korifiv1alpha1.CFServicePlan,
	osbapiClient osbapi.BrokerClient,
) (map[string]any, error) {
	log := logr.FromContextOrDiscard(ctx)

	lastOperation, err := osbapiClient.GetServiceBindingLastOperation(ctx, osbapi.GetServiceBindingLastOperationRequest{
		InstanceID: cfServiceBinding.Spec.Service.Name,
		BindingID:  cfServiceBinding.Name,
		GetLastOperationRequestParameters: osbapi.GetLastOperationRequestParameters{
			ServiceId: serviceOffering.Spec.BrokerCatalog.ID,
			PlanID:    servicePlan.Spec.BrokerCatalog.ID,
			Operation: cfServiceBinding.Status.BindingOperation,
		},
	})
	if err != nil {
		log.Error(err, "failed to get last operation", "operation", cfServiceBinding.Status.BindingOperation)
		return nil, k8s.NewNotReadyError().WithCause(err).WithReason("GetLastOperationFailed")
	}
	if lastOperation.State == "in progress" {
		log.Info("binding operation in progress", "operation", cfServiceBinding.Status.BindingOperation)
		return nil, k8s.NewNotReadyError().WithReason("BindingInProgress").WithRequeue()
	}

	if lastOperation.State == "failed" {
		log.Error(nil, "last operation has failed", "operation", cfServiceBinding.Status.BindingOperation, "description", lastOperation.Description)
		meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.BindingFailedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cfServiceBinding.Generation,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "BindingFailed",
			Message:            lastOperation.Description,
		})
		return nil, k8s.NewNotReadyError().WithReason("BindingFailed")
	}

	binding, err := osbapiClient.GetServiceBinding(ctx, osbapi.GetServiceBindingRequest{
		InstanceID: cfServiceBinding.Spec.Service.Name,
		BindingID:  cfServiceBinding.Name,
		ServiceId:  serviceOffering.Spec.BrokerCatalog.ID,
		PlanID:     servicePlan.Spec.BrokerCatalog.ID,
	})
	if err != nil {
		log.Error(err, "failed to get binding")
		return nil, err
	}

	return binding.Credentials, nil
}

func isBindRequested(binding *korifiv1alpha1.CFServiceBinding) bool {
	return meta.IsStatusConditionTrue(binding.Status.Conditions, korifiv1alpha1.BindingRequestedCondition)
}

func isFailed(binding *korifiv1alpha1.CFServiceBinding) bool {
	return meta.IsStatusConditionTrue(binding.Status.Conditions, korifiv1alpha1.BindingFailedCondition)
}

func isReconciled(binding *korifiv1alpha1.CFServiceBinding) bool {
	return binding.Status.Credentials.Name != "" && binding.Status.Binding.Name != ""
}

func (r *ManagedCredentialsReconciler) reconcileCredentials(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding, creds map[string]any) error {
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
		return err
	}
	cfServiceBinding.Status.Credentials.Name = credentialsSecret.Name

	bindingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceBinding.Name + "-sbio",
			Namespace: cfServiceBinding.Namespace,
		},
	}
	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, bindingSecret, func() error {
		bindingSecret.Type = corev1.SecretType(credentials.ServiceBindingSecretTypePrefix + korifiv1alpha1.ManagedType)
		bindingSecret.Data, err = credentials.GetServiceBindingIOSecretData(credentialsSecret)
		if err != nil {
			return err
		}

		return controllerutil.SetControllerReference(cfServiceBinding, bindingSecret, r.scheme)
	})
	if err != nil {
		log.Error(err, "failed to create binding secret")
		return err
	}

	cfServiceBinding.Status.Binding.Name = bindingSecret.Name

	return nil
}
