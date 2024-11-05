package osbapi

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Assets struct {
	k8sClient     client.Client
	rootNamespace string
}

func NewAssets(k8sClient client.Client, rootNamespace string) *Assets {
	return &Assets{
		k8sClient:     k8sClient,
		rootNamespace: rootNamespace,
	}
}

type ServiceInstanceAssets struct {
	ServiceBroker   *korifiv1alpha1.CFServiceBroker
	ServiceOffering *korifiv1alpha1.CFServiceOffering
	ServicePlan     *korifiv1alpha1.CFServicePlan
}

func (r *Assets) GetServiceInstanceAssets(ctx context.Context, serviceInstance *korifiv1alpha1.CFServiceInstance) (ServiceInstanceAssets, error) {
	servicePlan, err := r.getServicePlan(ctx, serviceInstance.Spec.PlanGUID)
	if err != nil {
		return ServiceInstanceAssets{}, err
	}

	serviceBroker, err := r.getServiceBroker(ctx, servicePlan.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel])
	if err != nil {
		return ServiceInstanceAssets{}, err
	}

	serviceOffering, err := r.getServiceOffering(ctx, servicePlan.Labels[korifiv1alpha1.RelServiceOfferingGUIDLabel])
	if err != nil {
		return ServiceInstanceAssets{}, err
	}

	return ServiceInstanceAssets{
		ServiceBroker:   serviceBroker,
		ServiceOffering: serviceOffering,
		ServicePlan:     servicePlan,
	}, nil
}

type ServiceBindingAssets struct {
	ServiceInstanceAssets
	ServiceInstance *korifiv1alpha1.CFServiceInstance
}

func (r *Assets) GetServiceBindingAssets(ctx context.Context, serviceBinding *korifiv1alpha1.CFServiceBinding) (ServiceBindingAssets, error) {
	serviceInstance := &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: serviceBinding.Namespace,
			Name:      serviceBinding.Spec.Service.Name,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceInstance), serviceInstance)
	if err != nil {
		return ServiceBindingAssets{}, err
	}

	serviceInstanceAssets, err := r.GetServiceInstanceAssets(ctx, serviceInstance)
	if err != nil {
		return ServiceBindingAssets{}, err
	}

	return ServiceBindingAssets{
		ServiceInstanceAssets: serviceInstanceAssets,
		ServiceInstance:       serviceInstance,
	}, nil
}

func (r *Assets) getServiceOffering(ctx context.Context, offeringGUID string) (*korifiv1alpha1.CFServiceOffering, error) {
	serviceOffering := &korifiv1alpha1.CFServiceOffering{
		ObjectMeta: metav1.ObjectMeta{
			Name:      offeringGUID,
			Namespace: r.rootNamespace,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceOffering), serviceOffering)
	if err != nil {
		return nil, fmt.Errorf("failed to get service offering %q: %w", offeringGUID, err)
	}

	return serviceOffering, nil
}

func (r *Assets) getServicePlan(ctx context.Context, planGUID string) (*korifiv1alpha1.CFServicePlan, error) {
	servicePlan := &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      planGUID,
			Namespace: r.rootNamespace,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(servicePlan), servicePlan)
	if err != nil {
		return nil, fmt.Errorf("failed to get service plan %q: %w", planGUID, err)
	}
	return servicePlan, nil
}

func (r *Assets) getServiceBroker(ctx context.Context, brokerGUID string) (*korifiv1alpha1.CFServiceBroker, error) {
	serviceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      brokerGUID,
			Namespace: r.rootNamespace,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)
	if err != nil {
		return nil, fmt.Errorf("failed to get service broker %q: %w", brokerGUID, err)
	}

	return serviceBroker, nil
}
