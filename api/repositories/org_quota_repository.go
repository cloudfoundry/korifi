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

func (r *OrgQuotaRepo) toResource(ctx context.Context, userClient client.WithWatch, cfOrgQuota *korifiv1alpha1.CFOrgQuota) (korifiv1alpha1.OrgQuotaResource, error) {
	organizations, err := r.GetRelationshipOrgs(ctx, userClient, cfOrgQuota.Name)
	if err != nil {
		return korifiv1alpha1.OrgQuotaResource{}, err
	}
	return korifiv1alpha1.OrgQuotaResource{
		OrgQuota: cfOrgQuota.Spec.OrgQuota,
		CFResource: korifiv1alpha1.CFResource{
			GUID: cfOrgQuota.Name,
		},
		Relationships: korifiv1alpha1.OrgQuotaRelationships{
			Organizations: organizations,
		},
	}, nil
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

	return r.toResource(ctx, userClient, cfOrgQuota)
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
		orgQuotaResource, err := r.toResource(ctx, userClient, &o)
		if err != nil {
			return []korifiv1alpha1.OrgQuotaResource{}, err
		}
		orgQuotaResources = append(orgQuotaResources, orgQuotaResource)
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

	return r.toResource(ctx, userClient, cfOrgQuota)
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

	for _, relOrg := range organizations.Data {
		actualCfOrg := new(korifiv1alpha1.CFOrg)
		err = userClient.Get(ctx, client.ObjectKey{Namespace: r.rootNamespace, Name: relOrg.GUID}, actualCfOrg)
		if err != nil {
			return korifiv1alpha1.ToManyRelationship{}, fmt.Errorf("failed to get org: %w", apierrors.FromK8sError(err, OrgResourceType))
		}
		err = k8s.PatchResource(ctx, userClient, actualCfOrg, func() {
			if actualCfOrg.Labels == nil {
				actualCfOrg.Labels = make(map[string]string)
			}
			actualCfOrg.Labels[korifiv1alpha1.RelOrgQuotaLabel] = guid
		})
		if err != nil {
			return korifiv1alpha1.ToManyRelationship{}, err
		}
	}

	return r.GetRelationshipOrgs(ctx, userClient, guid)
}

func (r *OrgQuotaRepo) GetRelationshipOrgs(ctx context.Context, userClient client.WithWatch, quotaGuid string) (korifiv1alpha1.ToManyRelationship, error) {
	cfOrgs := new(korifiv1alpha1.CFOrgList)
	err := userClient.List(ctx, cfOrgs, client.InNamespace(r.rootNamespace), client.MatchingLabels{
		korifiv1alpha1.RelOrgQuotaLabel: quotaGuid,
	})
	if err != nil {
		return korifiv1alpha1.ToManyRelationship{}, err
	}
	rels := make([]korifiv1alpha1.Relationship, len(cfOrgs.Items))
	for i, cfOrg := range cfOrgs.Items {
		rels[i] = korifiv1alpha1.Relationship{GUID: cfOrg.Name}
	}
	return korifiv1alpha1.ToManyRelationship{
		Data: rels,
	}, nil
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

	return r.toResource(ctx, userClient, actualCfOrgQuota)
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
