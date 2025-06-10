package repositories

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/google/uuid"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	SpaceResourceType = "Space"
)

type CreateSpaceMessage struct {
	Name             string
	OrganizationGUID string
}

type ListSpacesMessage struct {
	Names             []string
	GUIDs             []string
	OrganizationGUIDs []string
}

func (m *ListSpacesMessage) toListOptions(orgNs string, authorizedSpaceGuids []string) []ListOption {
	selectedGuids := authorizedSpaceGuids
	if len(m.GUIDs) != 0 {
		selectedGuids = slices.Collect(it.Filter(slices.Values(selectedGuids), func(guid string) bool {
			return slices.Contains(m.GUIDs, guid)
		}))
	}

	return []ListOption{
		InNamespace(orgNs),
		WithLabelStrictlyIn(korifiv1alpha1.GUIDLabelKey, selectedGuids),
		WithLabelIn(korifiv1alpha1.CFSpaceDisplayNameKey, tools.EncodeValuesToSha224(m.Names...)),
		WithLabel(korifiv1alpha1.ReadyLabelKey, string(metav1.ConditionTrue)),
	}
}

type DeleteSpaceMessage struct {
	GUID             string
	OrganizationGUID string
}

type PatchSpaceMetadataMessage struct {
	MetadataPatch
	GUID    string
	OrgGUID string
}

type SpaceRecord struct {
	Name             string
	GUID             string
	OrganizationGUID string
	Labels           map[string]string
	Annotations      map[string]string
	CreatedAt        time.Time
	UpdatedAt        *time.Time
	DeletedAt        *time.Time
}

func (r SpaceRecord) Relationships() map[string]string {
	return map[string]string{
		"organization": r.OrganizationGUID,
	}
}

type SpaceRepo struct {
	klient           Klient
	orgRepo          *OrgRepo
	nsPerms          *authorization.NamespacePermissions
	conditionAwaiter Awaiter[*korifiv1alpha1.CFSpace]
}

func NewSpaceRepo(
	klient Klient,
	orgRepo *OrgRepo,
	nsPerms *authorization.NamespacePermissions,
	conditionAwaiter Awaiter[*korifiv1alpha1.CFSpace],
) *SpaceRepo {
	return &SpaceRepo{
		klient:           klient,
		orgRepo:          orgRepo,
		nsPerms:          nsPerms,
		conditionAwaiter: conditionAwaiter,
	}
}

func (r *SpaceRepo) CreateSpace(ctx context.Context, info authorization.Info, message CreateSpaceMessage) (SpaceRecord, error) {
	_, err := r.orgRepo.GetOrg(ctx, info, message.OrganizationGUID)
	if err != nil {
		return SpaceRecord{}, fmt.Errorf("failed to get parent organization: %w", err)
	}

	cfSpace := &korifiv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: message.OrganizationGUID,
		},
		Spec: korifiv1alpha1.CFSpaceSpec{
			DisplayName: message.Name,
		},
	}
	err = r.klient.Create(ctx, cfSpace)
	if err != nil {
		return SpaceRecord{}, apierrors.FromK8sError(err, SpaceResourceType)
	}

	cfSpace, err = r.conditionAwaiter.AwaitCondition(ctx, r.klient, cfSpace, korifiv1alpha1.StatusConditionReady)
	if err != nil {
		return SpaceRecord{}, apierrors.FromK8sError(err, SpaceResourceType)
	}

	return cfSpaceToSpaceRecord(*cfSpace), nil
}

func (r *SpaceRepo) ListSpaces(ctx context.Context, authInfo authorization.Info, message ListSpacesMessage) ([]SpaceRecord, error) {
	authorizedOrgNamespaces, err := getAuthorizedOrgNamespaces(ctx, authInfo, r.nsPerms)
	if err != nil {
		return nil, err
	}

	orgNsList := slices.Collect(it.Filter(slices.Values(authorizedOrgNamespaces), func(ns string) bool {
		return tools.EmptyOrContains(message.OrganizationGUIDs, ns)
	}))

	authorizedSpaceNamespaces, err := r.nsPerms.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		return nil, err
	}

	cfSpaces := []korifiv1alpha1.CFSpace{}
	for _, org := range orgNsList {
		cfSpaceList := new(korifiv1alpha1.CFSpaceList)
		_, err = r.klient.List(ctx, cfSpaceList, message.toListOptions(org, slices.Collect(maps.Keys(authorizedSpaceNamespaces)))...)
		if k8serrors.IsForbidden(err) {
			continue
		}
		if err != nil {
			return nil, apierrors.FromK8sError(err, SpaceResourceType)
		}

		cfSpaces = append(cfSpaces, cfSpaceList.Items...)
	}

	return slices.Collect(it.Map(slices.Values(cfSpaces), cfSpaceToSpaceRecord)), nil
}

func (r *SpaceRepo) GetSpace(ctx context.Context, info authorization.Info, spaceGUID string) (SpaceRecord, error) {
	cfSpace := &korifiv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name: spaceGUID,
		},
	}
	err := r.klient.Get(ctx, cfSpace)
	if err != nil {
		return SpaceRecord{}, fmt.Errorf("failed to get space: %w", apierrors.FromK8sError(err, SpaceResourceType))
	}

	return cfSpaceToSpaceRecord(*cfSpace), nil
}

func cfSpaceToSpaceRecord(cfSpace korifiv1alpha1.CFSpace) SpaceRecord {
	return SpaceRecord{
		Name:             cfSpace.Spec.DisplayName,
		GUID:             cfSpace.Name,
		OrganizationGUID: cfSpace.Namespace,
		Annotations:      cfSpace.Annotations,
		Labels:           cfSpace.Labels,
		CreatedAt:        cfSpace.CreationTimestamp.Time,
		UpdatedAt:        getLastUpdatedTime(&cfSpace),
		DeletedAt:        golangTime(cfSpace.DeletionTimestamp),
	}
}

func (r *SpaceRepo) DeleteSpace(ctx context.Context, info authorization.Info, message DeleteSpaceMessage) error {
	err := r.klient.Delete(ctx, &korifiv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: message.OrganizationGUID,
		},
	})

	return apierrors.FromK8sError(err, SpaceResourceType)
}

func (r *SpaceRepo) PatchSpaceMetadata(ctx context.Context, authInfo authorization.Info, message PatchSpaceMetadataMessage) (SpaceRecord, error) {
	cfSpace := &korifiv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: message.OrgGUID,
			Name:      message.GUID,
		},
	}
	err := r.klient.Get(ctx, cfSpace)
	if err != nil {
		return SpaceRecord{}, fmt.Errorf("failed to get space: %w", apierrors.FromK8sError(err, SpaceResourceType))
	}

	err = r.klient.Patch(ctx, cfSpace, func() error {
		message.Apply(cfSpace)
		return nil
	})
	if err != nil {
		return SpaceRecord{}, apierrors.FromK8sError(err, SpaceResourceType)
	}

	return cfSpaceToSpaceRecord(*cfSpace), nil
}

func (r *SpaceRepo) GetDeletedAt(ctx context.Context, authInfo authorization.Info, spaceGUID string) (*time.Time, error) {
	space, err := r.GetSpace(ctx, authInfo, spaceGUID)
	if err != nil {
		return nil, err
	}
	return space.DeletedAt, nil
}
