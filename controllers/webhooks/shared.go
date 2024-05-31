package webhooks

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	MaxLabelLength = 63

	ImmutableFieldModificationErrorType = "ImmutableFieldModificationError"
	MissingRequredFieldErrorType        = "MissingRequiredFieldError"
	InvalidFieldValueErrorType          = "InvalidFieldValueError"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name NameValidator . NameValidator

type NameValidator interface {
	ValidateCreate(ctx context.Context, logger logr.Logger, namespace string, obj UniqueClientObject) error
	ValidateUpdate(ctx context.Context, logger logr.Logger, namespace string, oldObj, newObj UniqueClientObject) error
	ValidateDelete(ctx context.Context, logger logr.Logger, namespace string, obj UniqueClientObject) error
}

//counterfeiter:generate -o fake -fake-name NamespaceValidator . NamespaceValidator

type NamespaceValidator interface {
	ValidateOrgCreate(org korifiv1alpha1.CFOrg) error
	ValidateSpaceCreate(space korifiv1alpha1.CFSpace) error
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name NameRegistry . NameRegistry

type NameRegistry interface {
	RegisterName(ctx context.Context, namespace, name, ownerNamespace, ownerName string) error
	DeregisterName(ctx context.Context, namespace, name string) error
	TryLockName(ctx context.Context, namespace, name string) error
	UnlockName(ctx context.Context, namespace, name string) error
	CheckNameOwnership(ctx context.Context, namespace, name, ownerNamespace, ownerName string) (bool, error)
}

//counterfeiter:generate -o fake -fake-name UniqueClientObject . UniqueClientObject

type UniqueClientObject interface {
	client.Object
	UniqueName() string
	UniqueValidationErrorMessage() string
}
