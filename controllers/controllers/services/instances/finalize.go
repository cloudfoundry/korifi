package instances

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func FinalizeServiceBindings(
	ctx context.Context,
	k8sClient client.Client,
	serviceInstance *korifiv1alpha1.CFServiceInstance,
) error {
	bindings, err := getBindings(ctx, k8sClient, serviceInstance)
	if err != nil {
		return err
	}

	for _, binding := range bindings {
		if err = client.IgnoreNotFound(k8sClient.Delete(ctx, &binding)); err != nil {
			return err
		}
	}

	if len(bindings) > 0 {
		return k8s.NewNotReadyError().WithReason("BindingsAvailable").WithMessage(fmt.Sprintf("There are still %d bindings for the service instance", len(bindings)))
	}

	return nil
}

func getBindings(ctx context.Context, k8sClient client.Client, serviceInstance *korifiv1alpha1.CFServiceInstance) ([]korifiv1alpha1.CFServiceBinding, error) {
	serviceBindings := korifiv1alpha1.CFServiceBindingList{}
	if err := k8sClient.List(ctx, &serviceBindings,
		client.InNamespace(serviceInstance.Namespace),
		client.MatchingFields{shared.IndexServiceBindingServiceInstanceGUID: serviceInstance.Name},
	); err != nil {
		return nil, fmt.Errorf("failed to list bindings: %w", err)
	}

	return serviceBindings.Items, nil
}
