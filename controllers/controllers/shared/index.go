package shared

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	trinityv1alpha1 "github.tools.sap/neoCoreArchitecture/trinity-service-manager/controllers/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	IndexRouteDestinationAppName           = "destinationAppName"
	IndexRouteDomainQualifiedName          = "domainQualifiedName"
	IndexServiceBindingAppGUID             = "serviceBindingAppGUID"
	IndexServiceBindingServiceInstanceGUID = "serviceBindingServiceInstanceGUID"
	IndexAppTasks                          = "appTasks"
	IndexSpaceNamespaceName                = "spaceNamespace"
	IndexOrgNamespaceName                  = "orgNamespace"
	IndexServicePlanGUID                   = "servicePlanGUID"
	IndexServiceOfferingGUID               = "serviceOfferingGUID"
)

func SetupIndexWithManager(mgr manager.Manager) error {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), new(korifiv1alpha1.CFRoute), IndexRouteDestinationAppName, routeDestinationAppNameIndexFn)
	if err != nil {
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(context.Background(), new(korifiv1alpha1.CFRoute), IndexRouteDomainQualifiedName, routeDomainQualifiedNameIndexFn)
	if err != nil {
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(context.Background(), new(korifiv1alpha1.CFServiceBinding), IndexServiceBindingAppGUID, serviceBindingAppGUIDIndexFn)
	if err != nil {
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(context.Background(), new(korifiv1alpha1.CFServiceBinding), IndexServiceBindingServiceInstanceGUID, serviceBindingServiceInstanceGUIDIndexFn)
	if err != nil {
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(context.Background(), &korifiv1alpha1.CFTask{}, IndexAppTasks, func(object client.Object) []string {
		task := object.(*korifiv1alpha1.CFTask)
		return []string{task.Spec.AppRef.Name}
	})
	if err != nil {
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(context.Background(), &korifiv1alpha1.CFSpace{}, IndexSpaceNamespaceName, func(object client.Object) []string {
		space := object.(*korifiv1alpha1.CFSpace)
		return []string{space.Status.GUID}
	})
	if err != nil {
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(context.Background(), &korifiv1alpha1.CFOrg{}, IndexOrgNamespaceName, func(object client.Object) []string {
		org := object.(*korifiv1alpha1.CFOrg)
		return []string{org.Status.GUID}
	})
	if err != nil {
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(context.Background(), &trinityv1alpha1.CFServicePlan{}, IndexServicePlanGUID, func(object client.Object) []string {
		plan := object.(*trinityv1alpha1.CFServicePlan)
		return []string{plan.Spec.GUID}
	})
	if err != nil {
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(context.Background(), &trinityv1alpha1.CFServiceOffering{}, IndexServiceOfferingGUID, func(object client.Object) []string {
		offering := object.(*trinityv1alpha1.CFServiceOffering)
		return []string{offering.Spec.GUID}
	})
	if err != nil {
		return err
	}

	return nil
}

func routeDestinationAppNameIndexFn(rawObj client.Object) []string {
	route := rawObj.(*korifiv1alpha1.CFRoute)
	var destinationAppNames []string
	for _, destination := range route.Spec.Destinations {
		destinationAppNames = append(destinationAppNames, destination.AppRef.Name)
	}
	return destinationAppNames
}

func routeDomainQualifiedNameIndexFn(rawObj client.Object) []string {
	route := rawObj.(*korifiv1alpha1.CFRoute)
	return []string{route.Spec.DomainRef.Namespace + "." + route.Spec.DomainRef.Name}
}

func serviceBindingAppGUIDIndexFn(rawObj client.Object) []string {
	serviceBinding := rawObj.(*korifiv1alpha1.CFServiceBinding)
	return []string{serviceBinding.Spec.AppRef.Name}
}

func serviceBindingServiceInstanceGUIDIndexFn(rawObj client.Object) []string {
	serviceBinding := rawObj.(*korifiv1alpha1.CFServiceBinding)
	return []string{serviceBinding.Spec.Service.Name}
}
