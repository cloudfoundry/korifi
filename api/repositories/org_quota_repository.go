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
	OrgQuotaResourceType = "OrgQuota"
)

type CreateOrgQuotaMessage struct {
	Name        string
	Suspended   bool
	Labels      map[string]string
	Annotations map[string]string
}

type ListOrgQuotasMessage struct {
	Names []string
	GUIDs []string
}

type DeleteOrgQuotaMessage struct {
	GUID string
}

type PatchOrgQuotaMessage struct {
	MetadataPatch
	GUID string
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

func (r *OrgQuotaRepo) CreateOrgQuota(ctx context.Context, info authorization.Info, orgQuota korifiv1alpha1.OrgQuota) (korifiv1alpha1.OrgQuota, error) {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return korifiv1alpha1.OrgQuota{}, fmt.Errorf("failed to build user client: %w", err)
	}

	guid := uuid.NewString()
	orgQuota.GUID = guid

	cfOrgQuota := &korifiv1alpha1.CFOrgQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: r.rootNamespace,
		},
		Spec: orgQuota,
	}

	err = userClient.Create(ctx, cfOrgQuota)
	if err != nil {
		return korifiv1alpha1.OrgQuota{}, fmt.Errorf("failed to create cf org: %w", apierrors.FromK8sError(err, OrgQuotaResourceType))
	}

	return orgQuota, nil
}

func (r *OrgQuotaRepo) ListOrgQuotas(ctx context.Context, info authorization.Info, filter ListOrgQuotasMessage) ([]korifiv1alpha1.OrgQuota, error) {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return []korifiv1alpha1.OrgQuota{}, fmt.Errorf("failed to build user client: %w", err)
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

	var orgQuotas []korifiv1alpha1.OrgQuota
	for _, o := range Filter(cfOrgQuotaList.Items, preds...) {
		orgQuotas = append(orgQuotas, o.Spec)
	}
	return orgQuotas, nil
}

func (r *OrgQuotaRepo) GetOrgQuota(ctx context.Context, info authorization.Info, orgQuotaGUID string) (korifiv1alpha1.OrgQuota, error) {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return korifiv1alpha1.OrgQuota{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfOrgQuota := &korifiv1alpha1.CFOrgQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      orgQuotaGUID,
			Namespace: r.rootNamespace,
		},
	}

	err = userClient.Get(ctx, client.ObjectKeyFromObject(cfOrgQuota), cfOrgQuota)
	if err != nil {
		return korifiv1alpha1.OrgQuota{}, fmt.Errorf("failed to get org quota with id", "id", orgQuotaGUID)
	}

	return cfOrgQuota.Spec, nil
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

func (r *OrgQuotaRepo) PatchOrgQuota(ctx context.Context, authInfo authorization.Info, orgQuota korifiv1alpha1.OrgQuota) (korifiv1alpha1.OrgQuota, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return korifiv1alpha1.OrgQuota{}, fmt.Errorf("failed to build user client: %w", err)
	}

	actualCfOrgQuota := new(korifiv1alpha1.CFOrgQuota)
	err = userClient.Get(ctx, client.ObjectKey{Namespace: r.rootNamespace, Name: orgQuota.GUID}, actualCfOrgQuota)
	if err != nil {
		return korifiv1alpha1.OrgQuota{}, fmt.Errorf("failed to get org: %w", apierrors.FromK8sError(err, OrgQuotaResourceType))
	}

	err = k8s.PatchResource(ctx, userClient, actualCfOrgQuota, func() {
		actualCfOrgQuota.Spec = orgQuota
	})
	if err != nil {
		return korifiv1alpha1.OrgQuota{}, apierrors.FromK8sError(err, OrgQuotaResourceType)
	}

	return orgQuota, nil
}
