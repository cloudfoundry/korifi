package shared

import (
	"context"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	IndexRouteDestinationAppName              = "destinationAppName"
	IndexRouteDomainQualifiedName             = "domainQualifiedName"
	IndexServiceInstanceCredentialsSecretName = "serviceInstanceCredentialsSecretName"
	IndexServiceBindingAppGUID                = "serviceBindingAppGUID"
	IndexServiceBindingServiceInstanceGUID    = "serviceBindingServiceInstanceGUID"
	IndexAppTasks                             = "appTasks"
	IndexSpaceNamespaceName                   = "spaceNamespace"
	IndexOrgNamespaceName                     = "orgNamespace"
	IndexServiceBrokerCredentialsSecretName   = "serviceBrokerCredentialsSecretName"
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

	err = mgr.GetFieldIndexer().IndexField(context.Background(), &korifiv1alpha1.CFServiceInstance{}, IndexServiceInstanceCredentialsSecretName, func(object client.Object) []string {
		serviceInstance := object.(*korifiv1alpha1.CFServiceInstance)
		return []string{serviceInstance.Spec.SecretName}
	})
	if err != nil {
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(context.Background(), &korifiv1alpha1.CFServiceBroker{}, IndexServiceBrokerCredentialsSecretName, func(object client.Object) []string {
		serviceBroker := object.(*korifiv1alpha1.CFServiceBroker)
		return []string{serviceBroker.Spec.Credentials.Name}
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

func RemovePackageManagerKeys(src map[string]string, log logr.Logger) map[string]string {
	if src == nil {
		return src
	}

	dest := map[string]string{}
	for key, value := range src {
		if strings.HasPrefix(key, "meta.helm.sh/") {
			log.V(1).Info("skipping helm annotation propagation", "key", key)
			continue
		}

		if strings.HasPrefix(key, "kapp.k14s.io/") {
			log.V(1).Info("skipping kapp annotation propagation", "key", key)
			continue
		}

		dest[key] = value
	}

	return dest
}
