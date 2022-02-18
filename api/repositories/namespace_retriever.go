package repositories

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	CFAppsGVR = schema.GroupVersionResource{
		Group:    "workloads.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfapps",
	}

	CFBuildsGVR = schema.GroupVersionResource{
		Group:    "workloads.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfbuilds",
	}

	CFDomainsGVR = schema.GroupVersionResource{
		Group:    "networking.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfdomains",
	}

	CFDropletsGVR = schema.GroupVersionResource{
		Group:    "workloads.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfbuilds",
	}

	CFPackagesGVR = schema.GroupVersionResource{
		Group:    "workloads.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfpackages",
	}

	CFProcessesGVR = schema.GroupVersionResource{
		Group:    "workloads.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfprocesses",
	}

	CFRoutesGVR = schema.GroupVersionResource{
		Group:    "networking.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfroutes",
	}

	CFServiceBindingsGVR = schema.GroupVersionResource{
		Group:    "services.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfservicebindings",
	}

	CFServiceInstancesGVR = schema.GroupVersionResource{
		Group:    "services.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfserviceinstances",
	}

	ResourceMap = map[string]schema.GroupVersionResource{
		AppResourceType:             CFAppsGVR,
		BuildResourceType:           CFBuildsGVR,
		DropletResourceType:         CFDropletsGVR,
		DomainResourceType:          CFDomainsGVR,
		PackageResourceType:         CFPackagesGVR,
		ProcessResourceType:         CFProcessesGVR,
		RouteResourceType:           CFRoutesGVR,
		ServiceBindingResourceType:  CFServiceBindingsGVR,
		ServiceInstanceResourceType: CFServiceInstancesGVR,
	}
)

type NamespaceRetriever struct {
	client dynamic.Interface
}

func NewNamespaceRetriver(client dynamic.Interface) NamespaceRetriever {
	return NamespaceRetriever{
		client: client,
	}
}

func (nr NamespaceRetriever) NamespaceFor(ctx context.Context, resourceGUID, resourceType string) (string, error) {
	gvr, ok := ResourceMap[resourceType]
	if !ok {
		return "", fmt.Errorf("resource type %q unknown", resourceType)
	}

	opts := metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", resourceGUID),
	}

	list, err := nr.client.Resource(gvr).List(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("failed to list %v", resourceType)
	}

	if len(list.Items) == 0 {
		return "", NewNotFoundError(resourceType, nil)
	}

	if len(list.Items) > 1 {
		return "", fmt.Errorf("get-%s duplicate records exist", strings.ToLower(resourceType))
	}

	metadata := list.Items[0].Object["metadata"].(map[string]interface{})

	ns := metadata["namespace"].(string)

	if ns == "" {
		return "", fmt.Errorf("get-%s: resource is not namespace-scoped", strings.ToLower(resourceType))
	}

	return ns, nil
}
