package shared

import (
	"context"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	IndexRouteDestinationAppName = "destinationAppName"
	IndexServiceBindingAppGUID   = "serviceBindingAppGUID"
)

func SetupIndexWithManager(mgr manager.Manager) error {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &networkingv1alpha1.CFRoute{}, IndexRouteDestinationAppName, routeDestinationAppNameIndexFn)
	if err != nil {
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(context.Background(), &servicesv1alpha1.CFServiceBinding{}, IndexServiceBindingAppGUID, serviceBindingAppGUIDIndexFn)
	if err != nil {
		return err
	}

	return nil
}

func routeDestinationAppNameIndexFn(rawObj client.Object) []string {
	route := rawObj.(*networkingv1alpha1.CFRoute)
	var destinationAppNames []string
	for _, destination := range route.Spec.Destinations {
		destinationAppNames = append(destinationAppNames, destination.AppRef.Name)
	}
	return destinationAppNames
}

func serviceBindingAppGUIDIndexFn(rawObj client.Object) []string {
	serviceBinding := rawObj.(*servicesv1alpha1.CFServiceBinding)
	return []string{serviceBinding.Spec.AppRef.Name}
}
