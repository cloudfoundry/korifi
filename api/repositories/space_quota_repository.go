package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SpaceQuotaResourceType = "SpaceQuota"
)

type ListSpaceQuotasMessage struct {
	Names []string
	GUIDs []string
}

type SpaceQuotaRepo struct {
	rootNamespace     string
	userClientFactory authorization.UserK8sClientFactory
}

func NewSpaceQuotaRepo(
	rootNamespace string,
	userClientFactory authorization.UserK8sClientFactory,
) *SpaceQuotaRepo {
	return &SpaceQuotaRepo{
		rootNamespace:     rootNamespace,
		userClientFactory: userClientFactory,
	}
}

func toSpaceQuotaResource(cfSpaceQuota *korifiv1alpha1.CFSpaceQuota) korifiv1alpha1.SpaceQuotaResource {
	return korifiv1alpha1.SpaceQuotaResource{
		SpaceQuota: cfSpaceQuota.Spec.SpaceQuota,
		CFResource: korifiv1alpha1.CFResource{
			GUID: cfSpaceQuota.Name,
		},
	}
}

func (r *SpaceQuotaRepo) CreateSpaceQuota(ctx context.Context, info authorization.Info, spaceQuota korifiv1alpha1.SpaceQuota) (korifiv1alpha1.SpaceQuotaResource, error) {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return korifiv1alpha1.SpaceQuotaResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	guid := uuid.NewString()

	cfSpaceQuota := &korifiv1alpha1.CFSpaceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: r.rootNamespace,
		},
		Spec: korifiv1alpha1.SpaceQuotaSpec{
			SpaceQuota: spaceQuota,
		},
	}

	err = userClient.Create(ctx, cfSpaceQuota)
	if err != nil {
		return korifiv1alpha1.SpaceQuotaResource{}, fmt.Errorf("failed to create cf space quota: %w", apierrors.FromK8sError(err, SpaceQuotaResourceType))
	}

	return toSpaceQuotaResource(cfSpaceQuota), nil
}

func (r *SpaceQuotaRepo) ListSpaceQuotas(ctx context.Context, info authorization.Info, filter ListSpaceQuotasMessage) ([]korifiv1alpha1.SpaceQuotaResource, error) {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return []korifiv1alpha1.SpaceQuotaResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfSpaceQuotaList := new(korifiv1alpha1.CFSpaceQuotaList)
	err = userClient.List(ctx, cfSpaceQuotaList, client.InNamespace(r.rootNamespace))
	if err != nil {
		return nil, apierrors.FromK8sError(err, OrgQuotaResourceType)
	}

	preds := []func(korifiv1alpha1.CFSpaceQuota) bool{
		SetPredicate(filter.GUIDs, func(s korifiv1alpha1.CFSpaceQuota) string { return s.Name }),
		SetPredicate(filter.Names, func(s korifiv1alpha1.CFSpaceQuota) string { return s.Spec.Name }),
	}

	var spaceQuotas []korifiv1alpha1.SpaceQuotaResource
	for _, o := range Filter(cfSpaceQuotaList.Items, preds...) {
		spaceQuotas = append(spaceQuotas, toSpaceQuotaResource(&o))
	}
	return spaceQuotas, nil
}

func (r *SpaceQuotaRepo) GetSpaceQuota(ctx context.Context, info authorization.Info, spaceQuotaGUID string) (korifiv1alpha1.SpaceQuotaResource, error) {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return korifiv1alpha1.SpaceQuotaResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfSpaceQuota := &korifiv1alpha1.CFSpaceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spaceQuotaGUID,
			Namespace: r.rootNamespace,
		},
	}

	err = userClient.Get(ctx, client.ObjectKeyFromObject(cfSpaceQuota), cfSpaceQuota)
	if err != nil {
		return korifiv1alpha1.SpaceQuotaResource{}, fmt.Errorf("failed to get org quota with id %q: %w", spaceQuotaGUID, err)
	}

	return toSpaceQuotaResource(cfSpaceQuota), nil
}

func (r *SpaceQuotaRepo) DeleteSpaceQuota(ctx context.Context, info authorization.Info, spaceQuotaGUID string) error {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}
	err = userClient.Delete(ctx, &korifiv1alpha1.CFSpaceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spaceQuotaGUID,
			Namespace: r.rootNamespace,
		},
	})

	return apierrors.FromK8sError(err, SpaceQuotaResourceType)
}

func (r *SpaceQuotaRepo) AddSpaceQuotaRelationships(ctx context.Context, authInfo authorization.Info, guid string, spaces korifiv1alpha1.ToManyRelationship) (korifiv1alpha1.ToManyRelationship, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return korifiv1alpha1.ToManyRelationship{}, fmt.Errorf("failed to build user client: %w", err)
	}
	actualCfSpaceQuota := new(korifiv1alpha1.CFSpaceQuota)
	err = userClient.Get(ctx, client.ObjectKey{Namespace: r.rootNamespace, Name: guid}, actualCfSpaceQuota)
	if err != nil {
		return korifiv1alpha1.ToManyRelationship{}, fmt.Errorf("failed to get space quota: %w", apierrors.FromK8sError(err, OrgQuotaResourceType))
	}
	err = k8s.PatchResource(ctx, userClient, actualCfSpaceQuota, func() {
		(&actualCfSpaceQuota.Spec.Relationships.Spaces).Patch(spaces)
	})
	if err != nil {
		return korifiv1alpha1.ToManyRelationship{}, err
	}

	return actualCfSpaceQuota.Spec.Relationships.Spaces, nil

}

func (r *SpaceQuotaRepo) PatchSpaceQuota(ctx context.Context, authInfo authorization.Info, guid string, spaceQuotaPatch korifiv1alpha1.SpaceQuotaPatch) (korifiv1alpha1.SpaceQuotaResource, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return korifiv1alpha1.SpaceQuotaResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	actualCfSpaceQuota := new(korifiv1alpha1.CFSpaceQuota)
	err = userClient.Get(ctx, client.ObjectKey{Namespace: r.rootNamespace, Name: guid}, actualCfSpaceQuota)
	if err != nil {
		return korifiv1alpha1.SpaceQuotaResource{}, fmt.Errorf("failed to get org: %w", apierrors.FromK8sError(err, OrgQuotaResourceType))
	}

	err = k8s.PatchResource(ctx, userClient, actualCfSpaceQuota, func() {
		actualSpaceQuota := actualCfSpaceQuota.Spec.SpaceQuota
		actualSpaceQuota.Patch(spaceQuotaPatch)
	})
	if err != nil {
		return korifiv1alpha1.SpaceQuotaResource{}, apierrors.FromK8sError(err, SpaceQuotaResourceType)
	}

	return toSpaceQuotaResource(actualCfSpaceQuota), nil
}
