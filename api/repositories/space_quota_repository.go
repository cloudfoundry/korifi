package repositories

import (
	"context"
	"fmt"
	"time"

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

func (r *SpaceQuotaRepo) toSpaceQuotaResource(ctx context.Context, userClient client.WithWatch, cfSpaceQuota *korifiv1alpha1.CFSpaceQuota) (korifiv1alpha1.SpaceQuotaResource, error) {
	spaces, err := r.GetRelationshipSpaces(ctx, userClient, cfSpaceQuota.Name)
	if err != nil {
		return korifiv1alpha1.SpaceQuotaResource{}, err
	}
	return korifiv1alpha1.SpaceQuotaResource{
		SpaceQuota: cfSpaceQuota.Spec.SpaceQuota,
		CFResource: korifiv1alpha1.CFResource{
			GUID: cfSpaceQuota.Name,
		},
		Relationships: korifiv1alpha1.SpaceQuotaRelationships{
			Organization: korifiv1alpha1.ToOneRelationship{
				Data: korifiv1alpha1.Relationship{
					GUID: cfSpaceQuota.Labels[korifiv1alpha1.RelOrgLabel],
				},
			},
			Spaces: spaces,
		},
	}, nil
}

func (r *SpaceQuotaRepo) CreateSpaceQuota(ctx context.Context, info authorization.Info, spaceQuota korifiv1alpha1.SpaceQuota, relationships korifiv1alpha1.SpaceQuotaRelationships) (korifiv1alpha1.SpaceQuotaResource, error) {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return korifiv1alpha1.SpaceQuotaResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	guid := uuid.NewString()

	cfSpaceQuota := &korifiv1alpha1.CFSpaceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: r.rootNamespace,
			Labels: map[string]string{
				korifiv1alpha1.RelOrgLabel: relationships.Organization.Data.GUID,
			},
		},
		Spec: korifiv1alpha1.SpaceQuotaSpec{
			SpaceQuota: spaceQuota,
		},
	}

	err = userClient.Create(ctx, cfSpaceQuota)
	if err != nil {
		return korifiv1alpha1.SpaceQuotaResource{}, fmt.Errorf("failed to create cf space quota: %w", apierrors.FromK8sError(err, SpaceQuotaResourceType))
	}

	return r.toSpaceQuotaResource(ctx, userClient, cfSpaceQuota)
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
		spaceQuota, err := r.toSpaceQuotaResource(ctx, userClient, &o)
		if err != nil {
			return []korifiv1alpha1.SpaceQuotaResource{}, err
		}
		spaceQuotas = append(spaceQuotas, spaceQuota)
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

	return r.toSpaceQuotaResource(ctx, userClient, cfSpaceQuota)
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
	orgNamespace := actualCfSpaceQuota.Labels[korifiv1alpha1.RelOrgLabel]
	for _, relSpace := range spaces.Data {
		actualCfSpace := new(korifiv1alpha1.CFSpace)
		err = userClient.Get(ctx, client.ObjectKey{Namespace: orgNamespace, Name: relSpace.GUID}, actualCfSpace)
		if err != nil {
			return korifiv1alpha1.ToManyRelationship{}, fmt.Errorf("failed to get space: %w", apierrors.FromK8sError(err, SpaceResourceType))
		}
		err = k8s.PatchResource(ctx, userClient, actualCfSpace, func() {
			if actualCfSpace.Labels == nil {
				actualCfSpace.Labels = make(map[string]string)
			}
			actualCfSpace.Labels[korifiv1alpha1.RelSpaceQuotaLabel] = guid
		})
		if err != nil {
			return korifiv1alpha1.ToManyRelationship{}, err
		}
	}

	return r.GetRelationshipSpaces(ctx, userClient, guid)
}

func (r *SpaceQuotaRepo) GetRelationshipSpaces(ctx context.Context, userClient client.WithWatch, quotaGuid string) (korifiv1alpha1.ToManyRelationship, error) {
	cfSpaces := new(korifiv1alpha1.CFSpaceList)
	err := userClient.List(ctx, cfSpaces, client.InNamespace(r.rootNamespace), client.MatchingLabels{
		korifiv1alpha1.RelSpaceQuotaLabel: quotaGuid,
	})
	if err != nil {
		return korifiv1alpha1.ToManyRelationship{}, err
	}
	rels := make([]korifiv1alpha1.Relationship, len(cfSpaces.Items))
	for i, cfSpace := range cfSpaces.Items {
		rels[i] = korifiv1alpha1.Relationship{GUID: cfSpace.Name}
	}
	return korifiv1alpha1.ToManyRelationship{
		Data: rels,
	}, nil
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

	return r.toSpaceQuotaResource(ctx, userClient, actualCfSpaceQuota)
}

func (r *SpaceQuotaRepo) GetDeletedAt(ctx context.Context, authInfo authorization.Info, spaceQuotaGUID string) (*time.Time, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("get-deleted-at failed to build user client: %w", err)
	}

	cfSpaceQuota := new(korifiv1alpha1.CFSpaceQuota)
	err = userClient.Get(ctx, client.ObjectKey{Namespace: r.rootNamespace, Name: spaceQuotaGUID}, cfSpaceQuota)
	if err != nil {
		return nil, apierrors.FromK8sError(err, SpaceQuotaResourceType)
	}

	if cfSpaceQuota.GetDeletionTimestamp() != nil {
		return &cfSpaceQuota.GetDeletionTimestamp().Time, nil
	}
	return nil, nil
}
