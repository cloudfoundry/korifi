package shared

import (
	"context"

	networkingv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/networking/v1alpha1"
	servicesv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/services/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	IndexRouteDestinationAppName           = "destinationAppName"
	IndexServiceBindingAppGUID             = "serviceBindingAppGUID"
	IndexServiceBindingServiceInstanceGUID = "serviceBindingServiceInstanceGUID"
)

func SetupIndexWithManager(mgr manager.Manager) error {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), new(networkingv1alpha1.CFRoute), IndexRouteDestinationAppName, routeDestinationAppNameIndexFn)
	if err != nil {
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(context.Background(), new(servicesv1alpha1.CFServiceBinding), IndexServiceBindingAppGUID, serviceBindingAppGUIDIndexFn)
	if err != nil {
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(context.Background(), new(servicesv1alpha1.CFServiceBinding), IndexServiceBindingServiceInstanceGUID, serviceBindingServiceInstanceGUIDIndexFn)
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

func serviceBindingServiceInstanceGUIDIndexFn(rawObj client.Object) []string {
	serviceBinding := rawObj.(*servicesv1alpha1.CFServiceBinding)
	return []string{serviceBinding.Spec.Service.Name}
}
