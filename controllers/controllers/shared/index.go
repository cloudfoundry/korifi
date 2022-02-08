package shared

import (
	"context"

	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	DestinationAppName         = "destinationAppName"
	ServiceBindingAppGUIDIndex = "serviceBindingAppGUID"
)

func SetupIndexWithManager(mgr manager.Manager) error {
	if err := setupRouteIndex(mgr); err != nil {
		return err
	}
	if err := setupServiceBindingIndex(mgr); err != nil {
		return err
	}
	return nil
}

func setupRouteIndex(mgr manager.Manager) error {
	// Generate indexes for CFRoute on field spec.Destination.AppRef.Name for efficient querying.
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &networkingv1alpha1.CFRoute{}, DestinationAppName,
		func(rawObj client.Object) []string {
			route := rawObj.(*networkingv1alpha1.CFRoute)
			var destinationAppNames []string
			for _, destination := range route.Spec.Destinations {
				destinationAppNames = append(destinationAppNames, destination.AppRef.Name)
			}
			return destinationAppNames
		}); err != nil {
		return err
	}

	return nil
}

func setupServiceBindingIndex(mgr manager.Manager) error {
	// Generate indexes for CFServiceBinding on field serviceBinding.Spec.AppRef.Name for efficient querying.
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &servicesv1alpha1.CFServiceBinding{}, ServiceBindingAppGUIDIndex, func(rawObj client.Object) []string {
		serviceBinding := rawObj.(*servicesv1alpha1.CFServiceBinding)
		return []string{serviceBinding.Spec.AppRef.Name}
	}); err != nil {
		return err
	}

	return nil
}
