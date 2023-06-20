package coordination

import (
	"context"
	"crypto/sha1"
	"fmt"

	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"

	coordinationv1 "k8s.io/api/coordination/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	hashedNamePrefix = "n-"

	EntityTypeAnnotation = "coordination.cloudfoundry.org/entity-type"
	NamespaceAnnotation  = "coordination.cloudfoundry.org/namespace"
	NameAnnotation       = "coordination.cloudfoundry.org/name"
)

var (
	unlockedIdentity = "none"
	lockedIdentity   = "locked"
)

//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;patch;delete

type NameRegistry struct {
	client     client.Client
	entityType string
	logger     logr.Logger
}

func NewNameRegistry(client client.Client, entityType string) NameRegistry {
	return NameRegistry{
		client:     client,
		entityType: entityType,
		logger:     controllerruntime.Log.WithName("name-registry").WithValues("entityType", entityType),
	}
}

func (r NameRegistry) RegisterName(ctx context.Context, namespace, name string) error {
	logger := r.logger.WithName("register-name").WithValues("namespace", namespace, "name", name)

	hashedName := hashName(r.entityType, name)

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hashedName,
			Namespace: namespace,
			Annotations: map[string]string{
				EntityTypeAnnotation: r.entityType,
				NamespaceAnnotation:  namespace,
				NameAnnotation:       name,
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity: &unlockedIdentity,
		},
	}

	if err := r.client.Create(ctx, lease); err != nil {
		r.logger.Error(err, "failed-to-register-name")
		return fmt.Errorf("creating a lease failed: %w", err)
	}

	logger.V(1).Info("registered-name", "hashedName", hashedName)

	return nil
}

func (r NameRegistry) DeregisterName(ctx context.Context, namespace, name string) error {
	logger := r.logger.WithName("deregister-name").WithValues("namespace", namespace, "name", name)

	hashedName := hashName(r.entityType, name)
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hashedName,
			Namespace: namespace,
		},
	}
	if err := r.client.Delete(ctx, lease); client.IgnoreNotFound(err) != nil {
		logger.Error(err, "failed-to-deregister-name")
		return fmt.Errorf("deleting a lease failed: %w", err)
	}

	logger.V(1).Info("deregistered-name", "hashedName", hashedName)

	return nil
}

func (r NameRegistry) TryLockName(ctx context.Context, namespace, name string) error {
	logger := r.logger.WithName("try-lock-name").WithValues("namespace", namespace, "name", name)

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hashName(r.entityType, name),
			Namespace: namespace,
		},
	}
	jsonPatch := fmt.Sprintf(`[
    {"op":"test", "path":"/spec/holderIdentity", "value": "%s"},
    {"op":"replace", "path":"/spec/holderIdentity", "value": "%s"}
    ]`, unlockedIdentity, lockedIdentity)

	if err := retry.OnError(k8s.NewDefaultBackoff(),
		func(err error) bool { return k8serrors.IsNotFound(err) },
		func() error {
			err := r.client.Patch(ctx, lease, client.RawPatch(types.JSONPatchType, []byte(jsonPatch)))
			if err != nil {
				logger.Info("failed-to-patch-existing-lease", "reason", err)
			}
			return err
		}); err != nil {
		return fmt.Errorf("failed to acquire lock on lease: %w", err)
	}

	return nil
}

func (r NameRegistry) UnlockName(ctx context.Context, namespace, name string) error {
	logger := r.logger.WithName("try-lock-name").WithValues("namespace", namespace, "name", name)

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hashName(r.entityType, name),
			Namespace: namespace,
		},
	}
	jsonPatch := fmt.Sprintf(`[
    {"op":"test", "path":"/spec/holderIdentity", "value": "%s"},
    {"op":"replace", "path":"/spec/holderIdentity", "value": "%s"}
    ]`, lockedIdentity, unlockedIdentity)

	if err := r.client.Patch(ctx, lease, client.RawPatch(types.JSONPatchType, []byte(jsonPatch))); err != nil {
		logger.Error(err, "failed-to-unlock-lease")
		return fmt.Errorf("failed to release lock on lease: %w", err)
	}

	return nil
}

func hashName(entityType, name string) string {
	input := fmt.Sprintf("%s::%s", entityType, name)
	return fmt.Sprintf("%s%x", hashedNamePrefix, sha1.Sum([]byte(input)))
}

// check we implement the interface
var _ webhooks.NameRegistry = NameRegistry{}
