package webhooks

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	OrgPlacementErrorType      = "OrgPlacementError"
	OrgPlacementErrorMessage   = "Organization '%s' must be placed in the root 'cf' namespace"
	SpacePlacementErrorType    = "SpacePlacementError"
	SpacePlacementErrorMessage = "Organization '%s' does not exist for Space '%s'"
)

type PlacementValidator struct {
	client        client.Client
	rootNamespace string
}

func NewPlacementValidator(client client.Client, rootNamespace string) *PlacementValidator {
	return &PlacementValidator{client: client, rootNamespace: rootNamespace}
}

func (v PlacementValidator) ValidateOrgCreate(org korifiv1alpha1.CFOrg) *ValidationError {
	if org.Namespace != v.rootNamespace {
		return &ValidationError{
			Type:    OrgPlacementErrorType,
			Message: fmt.Sprintf(OrgPlacementErrorMessage, org.Spec.DisplayName),
		}
	}

	return nil
}

func (v PlacementValidator) ValidateSpaceCreate(space korifiv1alpha1.CFSpace) *ValidationError {
	cfOrg := korifiv1alpha1.CFOrg{}

	err := v.client.Get(context.Background(), types.NamespacedName{Name: space.Namespace, Namespace: v.rootNamespace}, &cfOrg)
	if err != nil {
		return &ValidationError{
			Type:    SpacePlacementErrorType,
			Message: fmt.Sprintf(SpacePlacementErrorMessage, space.Namespace, space.Spec.DisplayName),
		}
	}

	return nil
}
