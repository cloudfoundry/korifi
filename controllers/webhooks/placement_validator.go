package webhooks

import (
	"context"
	"fmt"

	workloads "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	OrgPlacementErrorType      = "OrgPlacementError"
	OrgPlacementErrorMessage   = "Organization '%s' must be placed in the root 'cf' namespace"
	SpacePlacementErrorMessage = "Organization '%s' does not exist for Space '%s'"
)

type PlacementValidator struct {
	client        client.Client
	rootNamespace string
}

func NewPlacementValidator(client client.Client, rootNamespace string) *PlacementValidator {
	return &PlacementValidator{client: client, rootNamespace: rootNamespace}
}

func (v PlacementValidator) ValidateOrgCreate(org workloads.CFOrg) error {
	if org.ObjectMeta.Namespace != v.rootNamespace {
		err := fmt.Errorf(OrgPlacementErrorMessage, org.Spec.DisplayName)
		return err
	}

	return nil
}

func (v PlacementValidator) ValidateSpaceCreate(space workloads.CFSpace) error {
	ctx := context.Background()
	cfOrg := workloads.CFOrg{}
	err := v.client.Get(ctx, types.NamespacedName{Name: space.ObjectMeta.Namespace, Namespace: v.rootNamespace}, &cfOrg)
	if err != nil {
		err = fmt.Errorf(SpacePlacementErrorMessage, space.ObjectMeta.Namespace, space.Spec.DisplayName)
		return err
	}
	return nil
}
