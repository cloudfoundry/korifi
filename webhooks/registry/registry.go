package registry

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	coordv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RegistryType string

const (
	OrgType         RegistryType = "Org"
	SpaceType       RegistryType = "Space"
	unclaimedHolder string       = "cf-name-registry--unclaimed"
	claimedHolder   string       = "cf-name-registry--claimed"
)

type Registrar struct {
	client client.Client
}

func NewRegistrar(client client.Client) *Registrar {
	return &Registrar{
		client: client,
	}
}

func (l *Registrar) TryRegister(ctx context.Context, registryType RegistryType, obj client.Object, name string) error {
	leaseName := getLeaseName(registryType, name)
	holder := unclaimedHolder
	now := metav1.NewMicroTime(time.Now())

	lease := coordv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: obj.GetNamespace(),
			Labels: map[string]string{
				"type": string(registryType),
				"name": name,
			},
		},
		Spec: coordv1.LeaseSpec{
			HolderIdentity: &holder,
			AcquireTime:    &now,
		},
	}

	err := l.client.Create(ctx, &lease)
	if err != nil {
		return fmt.Errorf("failed to create registry record: %w", err)
	}

	return nil
}

func (l *Registrar) TryClaimLease(ctx context.Context, registryType RegistryType, obj client.Object, name string) error {
	lease := &coordv1.Lease{}
	lease.Namespace = obj.GetNamespace()
	lease.Name = getLeaseName(registryType, name)

	jsonPatch := fmt.Sprintf(`
	[
	  {"op":"test", "path":"/spec/holderIdentity", "value": "%s"},
	  {"op":"replace", "path":"/spec/holderIdentity", "value": "%s"}
	]`, unclaimedHolder, claimedHolder)

	return l.client.Patch(ctx, lease, client.RawPatch(types.JSONPatchType, []byte(jsonPatch)))
}

func (l *Registrar) DeleteRecordFor(ctx context.Context, registryType RegistryType, namespace, name string) error {
	lease := &coordv1.Lease{}
	lease.Namespace = namespace
	lease.Name = getLeaseName(registryType, name)

	return l.client.Delete(ctx, lease)
}

func getLeaseName(registryType RegistryType, name string) string {
	plain := []byte(string(registryType) + "::" + name)
	sum := sha256.Sum256(plain)

	return fmt.Sprintf("r-%x", sum)
}
