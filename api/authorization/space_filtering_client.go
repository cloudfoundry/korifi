package authorization

import (
	"context"
	"maps"
	"slices"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewSpaceFilteringClient(
	client client.WithWatch,
	privilegedClient client.WithWatch,
	nsPerms *NamespacePermissions,
) SpaceFilteringClient {
	return SpaceFilteringClient{
		WithWatch:        client,
		privilegedClient: privilegedClient,
		nsPerms:          nsPerms,
	}
}

type SpaceFilteringClient struct {
	client.WithWatch
	nsPerms          *NamespacePermissions
	privilegedClient client.WithWatch
}

func (c SpaceFilteringClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	effectiveListOpts := &client.ListOptions{}
	for _, o := range opts {
		o.ApplyToList(effectiveListOpts)
	}

	if effectiveListOpts.Namespace != "" {
		return c.WithWatch.List(ctx, list, effectiveListOpts)
	}

	selector, err := c.buildLabelSelector(ctx, effectiveListOpts)
	if err != nil {
		return err
	}

	effectiveListOpts.LabelSelector = selector

	return c.privilegedClient.List(ctx, list, effectiveListOpts)
}

func (c SpaceFilteringClient) buildLabelSelector(ctx context.Context, listOpts *client.ListOptions) (labels.Selector, error) {
	namespaces, err := c.getAuthorizedSpaceNamespaces(ctx)
	if err != nil {
		return nil, err
	}

	if len(namespaces) == 0 {
		return k8s.MatchNotingSelector(), nil
	}

	selector := labels.NewSelector()
	namespaceRequirement, err := labels.NewRequirement(korifiv1alpha1.SpaceGUIDKey, selection.In, namespaces)
	if err != nil {
		return nil, err
	}
	selector = selector.Add(*namespaceRequirement)

	if listOpts.LabelSelector != nil {
		userRequirements, _ := listOpts.LabelSelector.Requirements()
		selector = selector.Add(userRequirements...)
	}

	return selector, nil
}

func (c SpaceFilteringClient) getAuthorizedSpaceNamespaces(ctx context.Context) ([]string, error) {
	authInfo, _ := InfoFromContext(ctx)
	authNs, err := c.nsPerms.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		return nil, err
	}

	return slices.Collect(maps.Keys(authNs)), nil
}
