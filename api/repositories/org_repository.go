package repositories

import (
	"context"
	"fmt"
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	OrgResourceType = "Org"
)

type CreateOrgMessage struct {
	Name        string
	Suspended   bool
	Labels      map[string]string
	Annotations map[string]string
}

type ListOrgsMessage struct {
	Names []string
	GUIDs []string
}

func (m *ListOrgsMessage) matches(org korifiv1alpha1.CFOrg) bool {
	return tools.EmptyOrContains(m.GUIDs, org.Name) &&
		tools.EmptyOrContains(m.Names, org.Spec.DisplayName) &&
		meta.IsStatusConditionTrue(org.Status.Conditions, korifiv1alpha1.StatusConditionReady)
}

type DeleteOrgMessage struct {
	GUID string
}

type PatchOrgMessage struct {
	MetadataPatch
	GUID string
	Name *string
}

func (p *PatchOrgMessage) Apply(org *korifiv1alpha1.CFOrg) {
	if p.Name != nil {
		org.Spec.DisplayName = *p.Name
	}
	p.MetadataPatch.Apply(org)
}

type OrgRecord struct {
	Name        string
	GUID        string
	Suspended   bool
	Labels      map[string]string
	Annotations map[string]string
	CreatedAt   time.Time
	UpdatedAt   *time.Time
	DeletedAt   *time.Time
}

func (r OrgRecord) Relationships() map[string]string {
	return map[string]string{}
}

type OrgRepo struct {
	rootNamespace    string
	klient           Klient
	nsPerms          *authorization.NamespacePermissions
	conditionAwaiter Awaiter[*korifiv1alpha1.CFOrg]
}

func NewOrgRepo(
	klient Klient,
	rootNamespace string,
	nsPerms *authorization.NamespacePermissions,
	conditionAwaiter Awaiter[*korifiv1alpha1.CFOrg],
) *OrgRepo {
	return &OrgRepo{
		klient:           klient,
		rootNamespace:    rootNamespace,
		nsPerms:          nsPerms,
		conditionAwaiter: conditionAwaiter,
	}
}

func (r *OrgRepo) CreateOrg(ctx context.Context, info authorization.Info, message CreateOrgMessage) (OrgRecord, error) {
	cfOrg := &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Name:        uuid.NewString(),
			Namespace:   r.rootNamespace,
			Labels:      message.Labels,
			Annotations: message.Annotations,
		},
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: message.Name,
		},
	}

	err := r.klient.Create(ctx, cfOrg)
	if err != nil {
		return OrgRecord{}, fmt.Errorf("failed to create cf org: %w", apierrors.FromK8sError(err, OrgResourceType))
	}

	cfOrg, err = r.conditionAwaiter.AwaitCondition(ctx, r.klient, cfOrg, korifiv1alpha1.StatusConditionReady)
	if err != nil {
		return OrgRecord{}, apierrors.FromK8sError(err, OrgResourceType)
	}

	return cfOrgToOrgRecord(*cfOrg), nil
}

func (r *OrgRepo) ListOrgs(ctx context.Context, info authorization.Info, message ListOrgsMessage) ([]OrgRecord, error) {
	authorizedNamespaces, err := r.nsPerms.GetAuthorizedOrgNamespaces(ctx, info)
	if err != nil {
		return nil, err
	}

	cfOrgList := new(korifiv1alpha1.CFOrgList)
	err = r.klient.List(ctx, cfOrgList, InNamespace(r.rootNamespace))
	if err != nil {
		return nil, apierrors.FromK8sError(err, OrgResourceType)
	}

	filteredOrgs := itx.FromSlice(cfOrgList.Items).Filter(func(org korifiv1alpha1.CFOrg) bool {
		return authorizedNamespaces[org.Name] && message.matches(org)
	})
	return slices.Collect(it.Map(filteredOrgs, cfOrgToOrgRecord)), nil
}

func (r *OrgRepo) GetOrg(ctx context.Context, info authorization.Info, orgGUID string) (OrgRecord, error) {
	orgRecords, err := r.ListOrgs(ctx, info, ListOrgsMessage{GUIDs: []string{orgGUID}})
	if err != nil {
		return OrgRecord{}, err
	}

	if len(orgRecords) == 0 {
		return OrgRecord{}, apierrors.NewNotFoundError(nil, OrgResourceType)
	}

	return orgRecords[0], nil
}

func (r *OrgRepo) DeleteOrg(ctx context.Context, info authorization.Info, message DeleteOrgMessage) error {
	err := r.klient.Delete(ctx, &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: r.rootNamespace,
		},
	})

	return apierrors.FromK8sError(err, OrgResourceType)
}

func (r *OrgRepo) PatchOrg(ctx context.Context, authInfo authorization.Info, message PatchOrgMessage) (OrgRecord, error) {
	cfOrg := &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      message.GUID,
		},
	}
	err := r.klient.Get(ctx, cfOrg)
	if err != nil {
		return OrgRecord{}, fmt.Errorf("failed to get org: %w", apierrors.FromK8sError(err, OrgResourceType))
	}

	err = r.klient.Patch(ctx, cfOrg, func() error {
		message.Apply(cfOrg)
		return nil
	})
	if err != nil {
		return OrgRecord{}, apierrors.FromK8sError(err, OrgResourceType)
	}

	return cfOrgToOrgRecord(*cfOrg), nil
}

func (r *OrgRepo) GetDeletedAt(ctx context.Context, authInfo authorization.Info, orgGUID string) (*time.Time, error) {
	cfOrg := &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      orgGUID,
		},
	}
	err := r.klient.Get(ctx, cfOrg)
	if err != nil {
		return nil, apierrors.FromK8sError(err, OrgResourceType)
	}

	record := cfOrgToOrgRecord(*cfOrg)

	if record.DeletedAt != nil {
		return record.DeletedAt, nil
	}

	_, err = r.GetOrg(ctx, authInfo, orgGUID)
	return nil, err
}

func cfOrgToOrgRecord(cfOrg korifiv1alpha1.CFOrg) OrgRecord {
	return OrgRecord{
		GUID:        cfOrg.Name,
		Name:        cfOrg.Spec.DisplayName,
		Suspended:   false,
		Labels:      cfOrg.Labels,
		Annotations: cfOrg.Annotations,
		CreatedAt:   cfOrg.CreationTimestamp.Time,
		UpdatedAt:   getLastUpdatedTime(&cfOrg),
		DeletedAt:   golangTime(cfOrg.DeletionTimestamp),
	}
}
