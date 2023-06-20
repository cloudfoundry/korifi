package webhooks

import (
	"context"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

const DuplicateNameErrorType = "DuplicateNameError"

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name NameRegistry . NameRegistry

type NameRegistry interface {
	RegisterName(ctx context.Context, namespace, name string) error
	DeregisterName(ctx context.Context, namespace, name string) error
	TryLockName(ctx context.Context, namespace, name string) error
	UnlockName(ctx context.Context, namespace, name string) error
}

type DuplicateValidator struct {
	nameRegistry NameRegistry
}

func NewDuplicateValidator(nameRegistry NameRegistry) *DuplicateValidator {
	return &DuplicateValidator{
		nameRegistry: nameRegistry,
	}
}

func (v DuplicateValidator) ValidateCreate(ctx context.Context, logger logr.Logger, namespace, newName, duplicateNameError string) *ValidationError {
	logger = logger.WithName("duplicateValidator.ValidateCreate")
	err := v.nameRegistry.RegisterName(ctx, namespace, newName)
	if err != nil {
		logger.Info("failed to register name during create",
			"name", newName,
			"namespace", namespace,
			"reason", err,
		)

		if k8serrors.IsAlreadyExists(err) {
			return &ValidationError{
				Type:    DuplicateNameErrorType,
				Message: duplicateNameError,
			}
		}

		return &ValidationError{
			Type:    UnknownErrorType,
			Message: UnknownErrorMessage,
		}
	}

	return nil
}

func (v DuplicateValidator) ValidateUpdate(ctx context.Context, logger logr.Logger, namespace, oldName, newName, duplicateNameError string) *ValidationError {
	if oldName == newName {
		return nil
	}

	logger = logger.
		WithName("duplicateValidator.ValidateUpdate").
		WithValues("namespace", namespace, "oldName", oldName, "newName", newName)

	err := v.nameRegistry.TryLockName(ctx, namespace, oldName)
	if err != nil {
		logger.Info("failed to acquire lock on old name",
			"reason", err,
		)

		return &ValidationError{
			Type:    UnknownErrorType,
			Message: UnknownErrorMessage,
		}
	}

	logger.V(1).Info("locked-old-name")

	err = v.nameRegistry.RegisterName(ctx, namespace, newName)
	if err != nil {
		// cannot register new name, so unlock old registry entry allowing future renames
		unlockErr := v.nameRegistry.UnlockName(ctx, namespace, oldName)
		if unlockErr != nil {
			// A locked registry entry will remain, so future name updates will fail until operator intervenes
			logger.Info("failed to release lock on old name",
				"name", oldName,
				"namespace", namespace,
				"reason", unlockErr,
			)
		}

		logger.Info("failed to register new name during update",
			"name", newName,
			"namespace", namespace,
			"reason", err,
		)

		if k8serrors.IsAlreadyExists(err) {
			return &ValidationError{
				Type:    DuplicateNameErrorType,
				Message: duplicateNameError,
			}
		}

		return &ValidationError{
			Type:    UnknownErrorType,
			Message: UnknownErrorMessage,
		}
	}
	logger.V(1).Info("registered-new-name")

	err = v.nameRegistry.DeregisterName(ctx, namespace, oldName)
	if err != nil {
		// We cannot unclaim the old name. It will remain claimed until an operator intervenes.
		logger.Info("failed to deregister old name during update",
			"name", oldName,
			"namespace", namespace,
			"reason", err,
		)
	}
	logger.V(1).Info("deregistered-old-name")

	return nil
}

func (v DuplicateValidator) ValidateDelete(ctx context.Context, logger logr.Logger, namespace, oldName string) *ValidationError {
	logger = logger.WithName("duplicateValidator.ValidateDelete")
	err := v.nameRegistry.DeregisterName(ctx, namespace, oldName)
	if err != nil {
		logger.Info("failed to deregister name during delete",
			"namespace", namespace,
			"name", oldName,
			"reason", err,
		)

		if k8serrors.IsNotFound(err) {
			return nil
		}

		return &ValidationError{
			Type:    UnknownErrorType,
			Message: UnknownErrorMessage,
		}
	}

	return nil
}
