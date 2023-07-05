package coordination

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	EntityTypeAnnotation     = "coordination.cloudfoundry.org/entity-type"
	NamespaceAnnotation      = "coordination.cloudfoundry.org/namespace"
	NameAnnotation           = "coordination.cloudfoundry.org/name"
	OwnerNamespaceAnnotation = "coordination.cloudfoundry.org/owner-namespace"
	OwnerNameAnnotation      = "coordination.cloudfoundry.org/owner-name"
)

var (
	unlockedIdentity = "none"
	lockedIdentity   = "locked"
)

//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;create;patch;delete;list;watch

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

func (r NameRegistry) RegisterName(ctx context.Context, namespace, name, ownerNamespace, ownerName string) error {
	logger := r.logger.WithName("register-name").WithValues("namespace", namespace, "name", name)

	if isDryRun(ctx, logger) {
		return nil
	}

	hashedName := HashName(r.entityType, name)

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hashedName,
			Namespace: namespace,
			Annotations: map[string]string{
				EntityTypeAnnotation:     r.entityType,
				NamespaceAnnotation:      namespace,
				NameAnnotation:           name,
				OwnerNamespaceAnnotation: ownerNamespace,
				OwnerNameAnnotation:      ownerName,
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

	if isDryRun(ctx, logger) {
		return nil
	}

	hashedName := HashName(r.entityType, name)
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

	if isDryRun(ctx, logger) {
		return nil
	}

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      HashName(r.entityType, name),
			Namespace: namespace,
		},
	}

	jsonPatch := fmt.Sprintf(`[
    {"op":"test", "path":"/spec/holderIdentity", "value": "%s"},
    {"op":"replace", "path":"/spec/holderIdentity", "value": "%s"}
    ]`, unlockedIdentity, lockedIdentity)

	if err := r.client.Patch(ctx, lease, client.RawPatch(types.JSONPatchType, []byte(jsonPatch))); err != nil {
		logger.Info("failed-to-patch-existing-lease", "reason", err)
		return fmt.Errorf("failed to acquire lock on lease: %w", err)
	}

	return nil
}

func (r NameRegistry) UnlockName(ctx context.Context, namespace, name string) error {
	logger := r.logger.WithName("try-lock-name").WithValues("namespace", namespace, "name", name)

	if isDryRun(ctx, logger) {
		return nil
	}

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      HashName(r.entityType, name),
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

func (r NameRegistry) CheckNameOwnership(ctx context.Context, namespace, name, ownerNamespace, ownerName string) (bool, error) {
	r.logger.V(1).Info("checking-name-ownership")

	var lease coordinationv1.Lease
	err := r.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: HashName(r.entityType, name)}, &lease)
	if err != nil {
		return false, err
	}

	return lease.Annotations[OwnerNamespaceAnnotation] == ownerNamespace && lease.Annotations[OwnerNameAnnotation] == ownerName, nil
}

func isDryRun(ctx context.Context, logger logr.Logger) bool {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return false
	}

	result := req.DryRun != nil && *req.DryRun
	if result {
		logger.V(1).Info("skipping dry-run requests")
	}

	return result
}

// check we implement the interface
var _ webhooks.NameRegistry = NameRegistry{}
