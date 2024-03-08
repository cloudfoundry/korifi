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
	OrgQuotaResourceType = "OrgQuota"
)

type ListOrgQuotasMessage struct {
	Names []string
	GUIDs []string
}

type OrgQuotaRepo struct {
	rootNamespace     string
	userClientFactory authorization.UserK8sClientFactory
	nsPerms           *authorization.NamespacePermissions
}

func NewOrgQuotaRepo(
	rootNamespace string,
	userClientFactory authorization.UserK8sClientFactory,
	nsPerms *authorization.NamespacePermissions,
) *OrgQuotaRepo {
	return &OrgQuotaRepo{
		rootNamespace:     rootNamespace,
		userClientFactory: userClientFactory,
		nsPerms:           nsPerms,
	}
}

func toResource(cfOrgQuota *korifiv1alpha1.CFOrgQuota) korifiv1alpha1.OrgQuotaResource {
	return korifiv1alpha1.OrgQuotaResource{
		OrgQuota: cfOrgQuota.Spec.OrgQuota,
		CFResource: korifiv1alpha1.CFResource{
			GUID: cfOrgQuota.Name,
		},
	}
}

func (r *OrgQuotaRepo) CreateOrgQuota(ctx context.Context, info authorization.Info, orgQuota korifiv1alpha1.OrgQuota) (korifiv1alpha1.OrgQuotaResource, error) {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return korifiv1alpha1.OrgQuotaResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	guid := uuid.NewString()

	cfOrgQuota := &korifiv1alpha1.CFOrgQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: r.rootNamespace,
		},
		Spec: korifiv1alpha1.OrgQuotaSpec{
			OrgQuota: orgQuota,
		},
	}

	err = userClient.Create(ctx, cfOrgQuota)
	if err != nil {
		return korifiv1alpha1.OrgQuotaResource{}, fmt.Errorf("failed to create cf org: %w", apierrors.FromK8sError(err, OrgQuotaResourceType))
	}

	return toResource(cfOrgQuota), nil
}

func (r *OrgQuotaRepo) ListOrgQuotas(ctx context.Context, info authorization.Info, filter ListOrgQuotasMessage) ([]korifiv1alpha1.OrgQuotaResource, error) {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return []korifiv1alpha1.OrgQuotaResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfOrgQuotaList := new(korifiv1alpha1.CFOrgQuotaList)
	err = userClient.List(ctx, cfOrgQuotaList, client.InNamespace(r.rootNamespace))
	if err != nil {
		return nil, apierrors.FromK8sError(err, OrgQuotaResourceType)
	}

	preds := []func(korifiv1alpha1.CFOrgQuota) bool{
		SetPredicate(filter.GUIDs, func(s korifiv1alpha1.CFOrgQuota) string { return s.Name }),
		SetPredicate(filter.Names, func(s korifiv1alpha1.CFOrgQuota) string { return s.Spec.Name }),
	}

	var orgQuotaResources []korifiv1alpha1.OrgQuotaResource
	for _, o := range Filter(cfOrgQuotaList.Items, preds...) {
		orgQuotaResources = append(orgQuotaResources, toResource(&o))
	}
	return orgQuotaResources, nil
}

func (r *OrgQuotaRepo) GetOrgQuota(ctx context.Context, info authorization.Info, orgQuotaGUID string) (korifiv1alpha1.OrgQuotaResource, error) {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return korifiv1alpha1.OrgQuotaResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfOrgQuota := &korifiv1alpha1.CFOrgQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      orgQuotaGUID,
			Namespace: r.rootNamespace,
		},
	}

	err = userClient.Get(ctx, client.ObjectKeyFromObject(cfOrgQuota), cfOrgQuota)
	if err != nil {
		return korifiv1alpha1.OrgQuotaResource{}, fmt.Errorf("failed to get org quota with id %q: %w", orgQuotaGUID, err)
	}

	return toResource(cfOrgQuota), nil
}

func (r *OrgQuotaRepo) DeleteOrgQuota(ctx context.Context, info authorization.Info, orgQuotaGUID string) error {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}
	err = userClient.Delete(ctx, &korifiv1alpha1.CFOrgQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      orgQuotaGUID,
			Namespace: r.rootNamespace,
		},
	})

	return apierrors.FromK8sError(err, OrgQuotaResourceType)
}

func (r *OrgQuotaRepo) AddOrgQuotaRelationships(ctx context.Context, authInfo authorization.Info, guid string, organizations korifiv1alpha1.ToManyRelationship) (korifiv1alpha1.ToManyRelationship, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return korifiv1alpha1.ToManyRelationship{}, fmt.Errorf("failed to build user client: %w", err)
	}
	actualCfOrgQuota := new(korifiv1alpha1.CFOrgQuota)
	err = userClient.Get(ctx, client.ObjectKey{Namespace: r.rootNamespace, Name: guid}, actualCfOrgQuota)
	if err != nil {
		return korifiv1alpha1.ToManyRelationship{}, fmt.Errorf("failed to get org quota: %w", apierrors.FromK8sError(err, OrgQuotaResourceType))
	}
	err = k8s.PatchResource(ctx, userClient, actualCfOrgQuota, func() {
		actualRelationships := actualCfOrgQuota.Spec.Relationships
		if actualRelationships == nil {
			actualRelationships = &korifiv1alpha1.OrgQuotaRelationships{}
			actualCfOrgQuota.Spec.Relationships = actualRelationships
		}
		actualRelationships.Organizations.Patch(organizations)
	})
	if err != nil {
		return korifiv1alpha1.ToManyRelationship{}, err
	}

	return actualCfOrgQuota.Spec.Relationships.Organizations, nil
}

func (r *OrgQuotaRepo) PatchOrgQuota(ctx context.Context, authInfo authorization.Info, guid string, orgQuotaPatch korifiv1alpha1.OrgQuotaPatch) (korifiv1alpha1.OrgQuotaResource, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return korifiv1alpha1.OrgQuotaResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	actualCfOrgQuota := new(korifiv1alpha1.CFOrgQuota)
	err = userClient.Get(ctx, client.ObjectKey{Namespace: r.rootNamespace, Name: guid}, actualCfOrgQuota)
	if err != nil {
		return korifiv1alpha1.OrgQuotaResource{}, fmt.Errorf("failed to get org: %w", apierrors.FromK8sError(err, OrgQuotaResourceType))
	}

	err = k8s.PatchResource(ctx, userClient, actualCfOrgQuota, func() {
		actualOrgQuota := actualCfOrgQuota.Spec.OrgQuota
		actualOrgQuota.Patch(orgQuotaPatch)
	})
	if err != nil {
		return korifiv1alpha1.OrgQuotaResource{}, apierrors.FromK8sError(err, OrgQuotaResourceType)
	}

	return toResource(actualCfOrgQuota), nil
}

func (r *OrgQuotaRepo) GetDeletedAt(ctx context.Context, authInfo authorization.Info, orgQuotaGUID string) (*time.Time, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("get-deleted-at failed to build user client: %w", err)
	}

	cfOrgQuota := new(korifiv1alpha1.CFOrgQuota)
	err = userClient.Get(ctx, client.ObjectKey{Namespace: r.rootNamespace, Name: orgQuotaGUID}, cfOrgQuota)
	if err != nil {
		return nil, apierrors.FromK8sError(err, OrgQuotaResourceType)
	}

	if cfOrgQuota.GetDeletionTimestamp() != nil {
		return &cfOrgQuota.GetDeletionTimestamp().Time, nil
	}
	return nil, nil
}
