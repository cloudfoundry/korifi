package repositories

import (
	"context"
	"fmt"

	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GUIDToNamespace struct {
	privilegedClient client.Client
}

func NewGUIDToNamespace(privilegedClient client.Client) GUIDToNamespace {
	return GUIDToNamespace{
		privilegedClient: privilegedClient,
	}
}

func (g GUIDToNamespace) GetNamespaceForServiceInstance(ctx context.Context, guid string) (string, error) {
	var list servicesv1alpha1.CFServiceInstanceList
	if err := g.privilegedClient.List(ctx, &list, client.MatchingFields{"metadata.name": guid}); err != nil {
		return "", fmt.Errorf("getNamespaceForServiceInstance: unexpected error: %w", err)
	}

	if len(list.Items) == 0 {
		return "", NewNotFoundError(ServiceInstanceResourceType, nil)
	}
	if len(list.Items) > 1 {
		return "", fmt.Errorf("duplicate service instances for guid %q", guid)
	}

	return list.Items[0].Namespace, nil
}
