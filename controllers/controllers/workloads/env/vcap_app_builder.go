package env

import (
	"context"
	"encoding/json"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VCAPApplicationEnvValueBuilder struct {
	k8sClient   client.Client
	extraValues map[string]any
}

func NewVCAPApplicationEnvValueBuilder(k8sClient client.Client, extraValues map[string]any) *VCAPApplicationEnvValueBuilder {
	return &VCAPApplicationEnvValueBuilder{
		k8sClient:   k8sClient,
		extraValues: extraValues,
	}
}

func (b *VCAPApplicationEnvValueBuilder) BuildEnvValue(ctx context.Context, cfApp *korifiv1alpha1.CFApp) (map[string][]byte, error) {
	space, err := b.getSpaceFromNamespace(ctx, cfApp.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving space for CFApp: %w", err)
	}
	org, err := b.getOrgFromNamespace(ctx, space.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving org for CFSpace: %w", err)
	}

	appURIs, err := b.getAppURIs(ctx, cfApp)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving app routes: %w", err)
	}

	vars := b.extraValues
	if vars == nil {
		vars = map[string]any{}
	}
	vars["application_id"] = cfApp.Name
	vars["application_name"] = cfApp.Spec.DisplayName
	vars["name"] = cfApp.Spec.DisplayName
	vars["organization_id"] = org.Name
	vars["organization_name"] = org.Spec.DisplayName
	vars["space_id"] = space.Name
	vars["space_name"] = space.Spec.DisplayName
	vars["uris"] = appURIs
	vars["application_uris"] = appURIs

	marshalledVars, _ := json.Marshal(vars)

	return map[string][]byte{
		"VCAP_APPLICATION": marshalledVars,
	}, nil
}

func (b *VCAPApplicationEnvValueBuilder) getAppURIs(ctx context.Context, cfApp *korifiv1alpha1.CFApp) ([]string, error) {
	var appRoutes korifiv1alpha1.CFRouteList
	err := b.k8sClient.List(
		ctx,
		&appRoutes,
		client.InNamespace(cfApp.Namespace),
		client.MatchingFields{shared.IndexRouteDestinationAppName: cfApp.Name},
	)
	if err != nil {
		return nil, err
	}

	uris := make([]string, len(appRoutes.Items))
	for i := range appRoutes.Items {
		uris[i] = appRoutes.Items[i].Status.URI
	}

	return uris, nil
}

func (b *VCAPApplicationEnvValueBuilder) getSpaceFromNamespace(ctx context.Context, ns string) (korifiv1alpha1.CFSpace, error) {
	spaces := korifiv1alpha1.CFSpaceList{}
	if err := b.k8sClient.List(ctx, &spaces, client.MatchingFields{
		shared.IndexSpaceNamespaceName: ns,
	}); err != nil {
		return korifiv1alpha1.CFSpace{}, fmt.Errorf("error listing cfSpaces: %w", err)
	}

	if len(spaces.Items) != 1 {
		return korifiv1alpha1.CFSpace{}, fmt.Errorf("expected a unique CFSpace for namespace %q, got %d", ns, len(spaces.Items))
	}

	return spaces.Items[0], nil
}

func (b *VCAPApplicationEnvValueBuilder) getOrgFromNamespace(ctx context.Context, ns string) (korifiv1alpha1.CFOrg, error) {
	orgs := korifiv1alpha1.CFOrgList{}
	if err := b.k8sClient.List(ctx, &orgs, client.MatchingFields{
		shared.IndexOrgNamespaceName: ns,
	}); err != nil {
		return korifiv1alpha1.CFOrg{}, fmt.Errorf("error listing cfOrgs: %w", err)
	}

	if len(orgs.Items) != 1 {
		return korifiv1alpha1.CFOrg{}, fmt.Errorf("expected a unique CFOrg for namespace %q, got %d", ns, len(orgs.Items))
	}

	return orgs.Items[0], nil
}
