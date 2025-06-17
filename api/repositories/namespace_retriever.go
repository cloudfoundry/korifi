package repositories

import (
	"context"
	"fmt"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfapps;cfbuilds;cfdomains;cforgs;cfpackages;cfprocesses;cfroutes;cfsecuritygroups;cfservicebindings;cfservicebrokers;cfserviceinstances;cfserviceofferings;cfserviceplans;cfspaces;cftasks,verbs=list

var (
	CFAppsGVR = schema.GroupVersionResource{
		Group:    "korifi.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfapps",
	}

	CFBuildsGVR = schema.GroupVersionResource{
		Group:    "korifi.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfbuilds",
	}

	CFDomainsGVR = schema.GroupVersionResource{
		Group:    "korifi.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfdomains",
	}

	CFDropletsGVR = schema.GroupVersionResource{
		Group:    "korifi.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfbuilds",
	}

	CFPackagesGVR = schema.GroupVersionResource{
		Group:    "korifi.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfpackages",
	}

	CFProcessesGVR = schema.GroupVersionResource{
		Group:    "korifi.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfprocesses",
	}

	CFRoutesGVR = schema.GroupVersionResource{
		Group:    "korifi.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfroutes",
	}

	CFServiceBindingsGVR = schema.GroupVersionResource{
		Group:    "korifi.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfservicebindings",
	}

	CFServiceInstancesGVR = schema.GroupVersionResource{
		Group:    "korifi.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfserviceinstances",
	}

	CFSpacesGVR = schema.GroupVersionResource{
		Group:    "korifi.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cfspaces",
	}

	CFTasksGVR = schema.GroupVersionResource{
		Group:    "korifi.cloudfoundry.org",
		Version:  "v1alpha1",
		Resource: "cftasks",
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
		SpaceResourceType:           CFSpacesGVR,
		TaskResourceType:            CFTasksGVR,
	}
)

type NamespaceRetriever struct {
	client dynamic.Interface
}

func NewNamespaceRetriever(client dynamic.Interface) NamespaceRetriever {
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
		return "", fmt.Errorf("failed to list %v: %w", resourceType, apierrors.FromK8sError(err, resourceType))
	}

	if len(list.Items) == 0 {
		return "", apierrors.NewNotFoundError(fmt.Errorf("resource %q not found", resourceGUID), resourceType)
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
