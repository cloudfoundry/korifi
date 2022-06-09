package webhooks

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/go-logr/logr"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name NameValidator . NameValidator

type NameValidator interface {
	ValidateCreate(ctx context.Context, logger logr.Logger, namespace, newName, duplicateNameError string) *ValidationError
	ValidateUpdate(ctx context.Context, logger logr.Logger, namespace, oldName, newName, duplicateNameError string) *ValidationError
	ValidateDelete(ctx context.Context, logger logr.Logger, namespace, oldName string) *ValidationError
}

//counterfeiter:generate -o fake -fake-name NamespaceValidator . NamespaceValidator

type NamespaceValidator interface {
	ValidateOrgCreate(org korifiv1alpha1.CFOrg) *ValidationError
	ValidateSpaceCreate(space korifiv1alpha1.CFSpace) *ValidationError
}
