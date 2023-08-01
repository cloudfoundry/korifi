package shared

import (
	"context"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	trinityv1alpha1 "github.tools.sap/neoCoreArchitecture/trinity-service-manager/controllers/api/v1alpha1"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	StatusConditionReady = "Ready"
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

// GetConditionOrSetAsUnknown is a helper function that retrieves the value of the provided conditionType, like
// "Succeeded" and returns the value: "True", "False", or "Unknown". If the value is not present, the pointer to the
// list of conditions provided to the function is used to add an entry to the list of Conditions with a value of
// "Unknown" and "Unknown" is returned
func GetConditionOrSetAsUnknown(conditions *[]metav1.Condition, conditionType string, generation int64) metav1.ConditionStatus {
	if conditionStatus := meta.FindStatusCondition(*conditions, conditionType); conditionStatus != nil {
		return conditionStatus.Status
	}

	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               conditionType,
		Status:             metav1.ConditionUnknown,
		Reason:             "Unknown",
		Message:            "Unknown",
		ObservedGeneration: generation,
	})

	return metav1.ConditionUnknown
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

func ObjectLogger(log logr.Logger, obj client.Object) logr.Logger {
	return log.WithValues("namespace", obj.GetNamespace(), "name", obj.GetName(), "logID", uuid.NewString())
}
