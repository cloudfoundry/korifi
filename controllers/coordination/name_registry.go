package coordination

import (
	"context"
	"crypto/sha1"
	"fmt"

	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/tools/k8s"

	coordinationv1 "k8s.io/api/coordination/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
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
}

func NewNameRegistry(client client.Client, entityType string) NameRegistry {
	return NameRegistry{
		client:     client,
		entityType: entityType,
	}
}

func (r NameRegistry) RegisterName(ctx context.Context, namespace, name string) error {
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hashName(r.entityType, name),
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
		return fmt.Errorf("creating a lease failed: %w", err)
	}

	return nil
}

func (r NameRegistry) DeregisterName(ctx context.Context, namespace, name string) error {
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hashName(r.entityType, name),
			Namespace: namespace,
		},
	}
	if err := r.client.Delete(ctx, lease); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("deleting a lease failed: %w", err)
	}

	return nil
}

func (r NameRegistry) TryLockName(ctx context.Context, namespace, name string) error {
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
			return r.client.Patch(ctx, lease, client.RawPatch(types.JSONPatchType, []byte(jsonPatch)))
		}); err != nil {
		return fmt.Errorf("failed to acquire lock on lease: %w", err)
	}

	return nil
}

func (r NameRegistry) UnlockName(ctx context.Context, namespace, name string) error {
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
