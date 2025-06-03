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

type SpaceFilteringOpts struct {
	nsPerms *NamespacePermissions
}

func NewSpaceFilteringOpts(nsPerms *NamespacePermissions) *SpaceFilteringOpts {
	return &SpaceFilteringOpts{
		nsPerms: nsPerms,
	}
}

func (o *SpaceFilteringOpts) Apply(ctx context.Context, opts ...client.ListOption) (*client.ListOptions, error) {
	effectiveListOpts := &client.ListOptions{}
	for _, o := range opts {
		o.ApplyToList(effectiveListOpts)
	}

	if effectiveListOpts.Namespace != "" {
		return effectiveListOpts, nil
	}

	selector, err := o.buildLabelSelector(ctx, effectiveListOpts)
	if err != nil {
		return nil, err
	}

	effectiveListOpts.LabelSelector = selector

	return effectiveListOpts, nil
}

func (o *SpaceFilteringOpts) buildLabelSelector(ctx context.Context, listOpts *client.ListOptions) (labels.Selector, error) {
	namespaces, err := o.getAuthorizedSpaceNamespaces(ctx)
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

func (o *SpaceFilteringOpts) getAuthorizedSpaceNamespaces(ctx context.Context) ([]string, error) {
	authInfo, _ := InfoFromContext(ctx)
	authNs, err := o.nsPerms.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		return nil, err
	}

	return slices.Collect(maps.Keys(authNs)), nil
}
