package managed

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"code.cloudfoundry.org/korifi/controllers/controllers/services/credentials"
	servicebindingv1beta1 "github.com/servicebinding/runtime/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

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
		return r.finalizeCFServiceBinding(ctx, cfServiceBinding)
	}

	cfServiceInstance := new(korifiv1alpha1.CFServiceInstance)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.Service.Name, Namespace: cfServiceBinding.Namespace}, cfServiceInstance)
	if err != nil {
		log.Info("service instance not found", "service-instance", cfServiceBinding.Spec.Service.Name, "error", err)
		return ctrl.Result{}, err
	}

	if cfServiceBinding.Labels == nil {
		cfServiceBinding.Labels = map[string]string{}
	}
	cfServiceBinding.Labels[korifiv1alpha1.PlanGUIDLabelKey] = cfServiceInstance.Spec.PlanGUID

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

	credentials, err := r.bind(ctx, cfServiceBinding, assets, osbapiClient)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.reconcileCredentials(ctx, cfServiceBinding, credentials)
	if err != nil {
		return ctrl.Result{}, err
	}

	sbServiceBinding, err := r.reconcileSBServiceBinding(ctx, cfServiceBinding)
	if err != nil {
		log.Info("error creating/updating servicebinding.io servicebinding", "reason", err)
		return ctrl.Result{}, err
	}

	if !isSbServiceBindingReady(sbServiceBinding) {
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("ServiceBindingNotReady")
	}

	return ctrl.Result{}, nil
}

func (r *ManagedBindingsReconciler) bind(
	ctx context.Context,
	cfServiceBinding *korifiv1alpha1.CFServiceBinding,
	assets osbapi.ServiceBindingAssets,
	osbapiClient osbapi.BrokerClient,
) (map[string]any, error) {
	if !isBindRequested(cfServiceBinding) {
		return r.requestBind(ctx, cfServiceBinding, assets, osbapiClient)
	}

	return r.pollBindOperation(ctx, cfServiceBinding, assets, osbapiClient)
}

func (r *ManagedBindingsReconciler) requestBind(
	ctx context.Context,
	cfServiceBinding *korifiv1alpha1.CFServiceBinding,
	assets osbapi.ServiceBindingAssets,
	osbapiClient osbapi.BrokerClient,
) (map[string]any, error) {
	log := logr.FromContextOrDiscard(ctx)

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

func (r *ManagedBindingsReconciler) pollBindOperation(
	ctx context.Context,
	cfServiceBinding *korifiv1alpha1.CFServiceBinding,
	assets osbapi.ServiceBindingAssets,
	osbapiClient osbapi.BrokerClient,
) (map[string]any, error) {
	log := logr.FromContextOrDiscard(ctx)

	lastOperation, err := osbapiClient.GetServiceBindingLastOperation(ctx, osbapi.GetServiceBindingLastOperationRequest{
		InstanceID: cfServiceBinding.Spec.Service.Name,
		BindingID:  cfServiceBinding.Name,
		GetLastOperationRequestParameters: osbapi.GetLastOperationRequestParameters{
			ServiceId: assets.ServiceOffering.Spec.BrokerCatalog.ID,
			PlanID:    assets.ServicePlan.Spec.BrokerCatalog.ID,
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
		ServiceId:  assets.ServiceOffering.Spec.BrokerCatalog.ID,
		PlanID:     assets.ServicePlan.Spec.BrokerCatalog.ID,
	})
	if err != nil {
		log.Error(err, "failed to get binding")
		return nil, err
	}

	return binding.Credentials, nil
}

func (r *ManagedBindingsReconciler) reconcileCredentials(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding, creds map[string]any) error {
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

func (r *ManagedBindingsReconciler) finalizeCFServiceBinding(
	ctx context.Context,
	serviceBinding *korifiv1alpha1.CFServiceBinding,
) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("finalize-managed-service-binding")

	if controllerutil.RemoveFinalizer(serviceBinding, korifiv1alpha1.CFServiceBindingFinalizerName) {
		log.V(1).Info("finalizer removed")
	}
	return ctrl.Result{}, nil
}

func (r *ManagedBindingsReconciler) reconcileSBServiceBinding(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (*servicebindingv1beta1.ServiceBinding, error) {
	sbServiceBinding := r.toSBServiceBinding(cfServiceBinding)

	_, err := controllerutil.CreateOrPatch(ctx, r.k8sClient, sbServiceBinding, func() error {
		sbServiceBinding.Spec.Name = getSBServiceBindingName(cfServiceBinding)

		return controllerutil.SetControllerReference(cfServiceBinding, sbServiceBinding, r.scheme)
	})
	if err != nil {
		return nil, err
	}

	return sbServiceBinding, nil
}

func (r *ManagedBindingsReconciler) toSBServiceBinding(cfServiceBinding *korifiv1alpha1.CFServiceBinding) *servicebindingv1beta1.ServiceBinding {
	return &servicebindingv1beta1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cf-binding-%s", cfServiceBinding.Name),
			Namespace: cfServiceBinding.Namespace,
			Labels: map[string]string{
				korifiv1alpha1.ServiceBindingGUIDLabel:           cfServiceBinding.Name,
				korifiv1alpha1.CFAppGUIDLabelKey:                 cfServiceBinding.Spec.AppRef.Name,
				korifiv1alpha1.ServiceCredentialBindingTypeLabel: "app",
			},
		},
		Spec: servicebindingv1beta1.ServiceBindingSpec{
			Type: korifiv1alpha1.ManagedType,
			Workload: servicebindingv1beta1.ServiceBindingWorkloadReference{
				APIVersion: "apps/v1",
				Kind:       "StatefulSet",
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						korifiv1alpha1.CFAppGUIDLabelKey: cfServiceBinding.Spec.AppRef.Name,
					},
				},
			},
			Service: servicebindingv1beta1.ServiceBindingServiceReference{
				APIVersion: "korifi.cloudfoundry.org/v1alpha1",
				Kind:       "CFServiceBinding",
				Name:       cfServiceBinding.Name,
			},
		},
	}
}

func getSBServiceBindingName(cfServiceBinding *korifiv1alpha1.CFServiceBinding) string {
	if cfServiceBinding.Spec.DisplayName != nil {
		return *cfServiceBinding.Spec.DisplayName
	}

	return cfServiceBinding.Status.Binding.Name
}

func isSbServiceBindingReady(sbServiceBinding *servicebindingv1beta1.ServiceBinding) bool {
	readyCondition := meta.FindStatusCondition(sbServiceBinding.Status.Conditions, "Ready")
	if readyCondition == nil {
		return false
	}

	if readyCondition.Status != metav1.ConditionTrue {
		return false
	}

	return sbServiceBinding.Generation == sbServiceBinding.Status.ObservedGeneration
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
