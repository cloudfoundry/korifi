package workloads

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

var ErrorDuplicateName = errors.New("name already used in namespace")

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

func (v DuplicateValidator) ValidateCreate(ctx context.Context, logger logr.Logger, namespace, newName string) error {
	logger = logger.WithName("duplicateValidator.ValidateCreate")
	err := v.nameRegistry.RegisterName(ctx, namespace, newName)
	if k8serrors.IsAlreadyExists(err) {
		logger.Info("failed to register name during create",
			"error", err,
			"name", newName,
			"namespace", namespace,
		)
		return ErrorDuplicateName
	}
	if err != nil {
		return err
	}

	return nil
}

func (v DuplicateValidator) ValidateUpdate(ctx context.Context, logger logr.Logger, namespace, oldName, newName string) error {
	logger = logger.WithName("duplicateValidator.ValidateUpdate")
	if oldName == newName {
		return nil
	}

	err := v.nameRegistry.TryLockName(ctx, namespace, oldName)
	if err != nil {
		logger.Info("failed to acquire lock on old name",
			"error", err,
			"name", oldName,
			"namespace", namespace,
		)
		return err
	}

	err = v.nameRegistry.RegisterName(ctx, namespace, newName)
	if err != nil {
		// cannot register new name, so unlock old registry entry allowing future renames
		unlockErr := v.nameRegistry.UnlockName(ctx, namespace, oldName)
		if unlockErr != nil {
			// A locked registry entry will remain, so future name updates will fail until operator intervenes
			logger.Error(unlockErr, "failed to release lock on old name",
				"name", oldName,
				"namespace", namespace,
			)
		}

		logger.Info("failed to register new name during update",
			"error", err,
			"name", newName,
			"namespace", namespace,
		)

		if k8serrors.IsAlreadyExists(err) {
			return ErrorDuplicateName
		}

		return err
	}

	err = v.nameRegistry.DeregisterName(ctx, namespace, oldName)
	if err != nil {
		// We cannot unclaim the old name. It will remain claimed until an operator intervenes.
		logger.Error(err, "failed to deregister old name during update",
			"name", oldName,
			"namespace", namespace,
		)
	}

	return nil
}

func (v DuplicateValidator) ValidateDelete(ctx context.Context, logger logr.Logger, namespace, oldName string) error {
	logger = logger.WithName("duplicateValidator.ValidateDelete")
	err := v.nameRegistry.DeregisterName(ctx, namespace, oldName)
	if k8serrors.IsNotFound(err) {
		logger.Info("cannot deregister name: registry entry for name not found",
			"namespace", namespace,
			"name", oldName,
		)
		return nil
	}

	if err != nil {
		logger.Error(err, "failed to deregister name during delete",
			"namespace", namespace,
			"name", oldName,
		)
		return err
	}

	return nil
}
