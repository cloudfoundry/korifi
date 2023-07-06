package webhooks

import (
	"context"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const DuplicateNameErrorType = "DuplicateNameError"

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

type DuplicateValidator struct {
	nameRegistry NameRegistry
}

func NewDuplicateValidator(nameRegistry NameRegistry) *DuplicateValidator {
	return &DuplicateValidator{
		nameRegistry: nameRegistry,
	}
}

func (v DuplicateValidator) ValidateCreate(ctx context.Context, logger logr.Logger, namespace string, obj UniqueClientObject) error {
	logger = logger.WithName("duplicateValidator.ValidateCreate")
	err := v.nameRegistry.RegisterName(ctx, namespace, obj.UniqueName(), obj.GetNamespace(), obj.GetName())
	if err != nil {
		logger.Info("failed to register name during create",
			"name", obj.UniqueName(),
			"namespace", namespace,
			"reason", err,
		)

		if k8serrors.IsAlreadyExists(err) {
			return duplicateError(obj)
		}

		return unknownError()
	}

	return nil
}

func (v DuplicateValidator) ValidateUpdate(ctx context.Context, logger logr.Logger, namespace string, oldObj, obj UniqueClientObject) error {
	if oldObj.UniqueName() == obj.UniqueName() {
		return nil
	}

	logger = logger.
		WithName("duplicateValidator.ValidateUpdate").
		WithValues("namespace", namespace, "oldName", oldObj.UniqueName(), "newName", obj.UniqueName())

	err := v.nameRegistry.TryLockName(ctx, namespace, oldObj.UniqueName())
	if err != nil {
		logger.Info("failed to acquire lock on old name", "reason", err)

		if k8serrors.IsNotFound(err) {
			isOwned, ownershipErr := v.nameRegistry.CheckNameOwnership(ctx, namespace, obj.UniqueName(), obj.GetNamespace(), obj.GetName())
			if ownershipErr != nil {
				logger.Error(ownershipErr, "failed to check ownership on new name")
				return unknownError()
			}

			if isOwned {
				logger.Info("unique name is already owned by updated object",
					"name", obj.UniqueName(),
					"updatedObjectKind", obj.GetObjectKind(),
					"object", client.ObjectKeyFromObject(obj),
				)
				return nil
			}
		}

		return unknownError()
	}

	logger.V(1).Info("locked-old-name")

	err = v.nameRegistry.RegisterName(ctx, namespace, obj.UniqueName(), obj.GetNamespace(), obj.GetName())
	if err != nil {
		// cannot register new name, so unlock old registry entry allowing future renames
		unlockErr := v.nameRegistry.UnlockName(ctx, namespace, oldObj.UniqueName())
		if unlockErr != nil {
			// A locked registry entry will remain, so future name updates will fail until operator intervenes
			logger.Info("failed to release lock on old name",
				"reason", unlockErr,
			)
		}

		logger.Info("failed to register new name during update",
			"reason", err,
		)

		if k8serrors.IsAlreadyExists(err) {
			return duplicateError(obj)
		}

		return unknownError()
	}
	logger.V(1).Info("registered-new-name")

	err = v.nameRegistry.DeregisterName(ctx, namespace, oldObj.UniqueName())
	if err != nil {
		// We cannot unclaim the old name. It will remain claimed until an operator intervenes.
		logger.Info("failed to deregister old name during update",
			"reason", err,
		)
	}
	logger.V(1).Info("deregistered-old-name")

	return nil
}

func (v DuplicateValidator) ValidateDelete(ctx context.Context, logger logr.Logger, namespace string, obj UniqueClientObject) error {
	logger = logger.WithName("duplicateValidator.ValidateDelete")
	err := v.nameRegistry.DeregisterName(ctx, namespace, obj.UniqueName())
	if err != nil {
		logger.Info("failed to deregister name during delete",
			"namespace", namespace,
			"name", obj.UniqueName(),
			"reason", err,
		)

		if k8serrors.IsNotFound(err) {
			return nil
		}

		return unknownError()
	}

	return nil
}

func duplicateError(obj UniqueClientObject) error {
	return ValidationError{
		Type:    DuplicateNameErrorType,
		Message: obj.UniqueValidationErrorMessage(),
	}.ExportJSONError()
}

func unknownError() error {
	return ValidationError{
		Type:    UnknownErrorType,
		Message: UnknownErrorMessage,
	}.ExportJSONError()
}

// check interface is implemented correctly
var _ NameValidator = DuplicateValidator{}
