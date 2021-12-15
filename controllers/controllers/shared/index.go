package shared

import (
	"context"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const DestinationAppName = "destinationAppName"

func SetupIndexWithManager(mgr manager.Manager) error {

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
