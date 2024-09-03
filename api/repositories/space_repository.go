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
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/google/uuid"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SpacePrefix       = "cf-space-"
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

func (m *ListSpacesMessage) matches(space korifiv1alpha1.CFSpace) bool {
	return meta.IsStatusConditionTrue(space.Status.Conditions, korifiv1alpha1.StatusConditionReady) &&
		tools.EmptyOrContains(m.GUIDs, space.Name) &&
		tools.EmptyOrContains(m.Names, space.Spec.DisplayName)
}

func (m *ListSpacesMessage) matchesNamespace(ns string) bool {
	return tools.EmptyOrContains(m.OrganizationGUIDs, ns)
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
	orgRepo            *OrgRepo
	namespaceRetriever NamespaceRetriever
	userClientFactory  authorization.UserK8sClientFactory
	nsPerms            *authorization.NamespacePermissions
	conditionAwaiter   Awaiter[*korifiv1alpha1.CFSpace]
}

func NewSpaceRepo(
	namespaceRetriever NamespaceRetriever,
	orgRepo *OrgRepo,
	userClientFactory authorization.UserK8sClientFactory,
	nsPerms *authorization.NamespacePermissions,
	conditionAwaiter Awaiter[*korifiv1alpha1.CFSpace],
) *SpaceRepo {
	return &SpaceRepo{
		orgRepo:            orgRepo,
		namespaceRetriever: namespaceRetriever,
		userClientFactory:  userClientFactory,
		nsPerms:            nsPerms,
		conditionAwaiter:   conditionAwaiter,
	}
}

func (r *SpaceRepo) CreateSpace(ctx context.Context, info authorization.Info, message CreateSpaceMessage) (SpaceRecord, error) {
	_, err := r.orgRepo.GetOrg(ctx, info, message.OrganizationGUID)
	if err != nil {
		return SpaceRecord{}, fmt.Errorf("failed to get parent organization: %w", err)
	}

	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return SpaceRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfSpace := &korifiv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SpacePrefix + uuid.NewString(),
			Namespace: message.OrganizationGUID,
		},
		Spec: korifiv1alpha1.CFSpaceSpec{
			DisplayName: message.Name,
		},
	}
	err = userClient.Create(ctx, cfSpace)
	if err != nil {
		return SpaceRecord{}, apierrors.FromK8sError(err, SpaceResourceType)
	}

	cfSpace, err = r.conditionAwaiter.AwaitCondition(ctx, userClient, cfSpace, korifiv1alpha1.StatusConditionReady)
	if err != nil {
		return SpaceRecord{}, apierrors.FromK8sError(err, SpaceResourceType)
	}

	return cfSpaceToSpaceRecord(*cfSpace), nil
}

func (r *SpaceRepo) ListSpaces(ctx context.Context, authInfo authorization.Info, message ListSpacesMessage) ([]SpaceRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []SpaceRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	authorizedOrgNamespaces, err := authorizedOrgNamespaces(ctx, authInfo, r.nsPerms)
	if err != nil {
		return nil, err
	}

	authorizedSpaceNamespaces, err := r.nsPerms.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		return nil, err
	}

	orgNsList := authorizedOrgNamespaces.Filter(message.matchesNamespace).Collect()
	cfSpaces := []korifiv1alpha1.CFSpace{}
	for _, org := range orgNsList {
		cfSpaceList := new(korifiv1alpha1.CFSpaceList)
		err = userClient.List(ctx, cfSpaceList, client.InNamespace(org))
		if k8serrors.IsForbidden(err) {
			continue
		}
		if err != nil {
			return nil, apierrors.FromK8sError(err, SpaceResourceType)
		}

		cfSpaces = append(cfSpaces, cfSpaceList.Items...)
	}

	filteredSpaces := itx.FromSlice(cfSpaces).Filter(func(s korifiv1alpha1.CFSpace) bool {
		return authorizedSpaceNamespaces[s.Name] && message.matches(s)
	})

	return slices.Collect(it.Map(filteredSpaces, cfSpaceToSpaceRecord)), nil
}

func (r *SpaceRepo) GetSpace(ctx context.Context, info authorization.Info, spaceGUID string) (SpaceRecord, error) {
	ns, err := r.namespaceRetriever.NamespaceFor(ctx, spaceGUID, SpaceResourceType)
	if err != nil {
		return SpaceRecord{}, err
	}

	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return SpaceRecord{}, fmt.Errorf("get-space failed to build user client: %w", err)
	}

	cfSpace := &korifiv1alpha1.CFSpace{}
	err = userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: spaceGUID}, cfSpace)
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
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	err = userClient.Delete(ctx, &korifiv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: message.OrganizationGUID,
		},
	})

	return apierrors.FromK8sError(err, SpaceResourceType)
}

func (r *SpaceRepo) PatchSpaceMetadata(ctx context.Context, authInfo authorization.Info, message PatchSpaceMetadataMessage) (SpaceRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return SpaceRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfSpace := new(korifiv1alpha1.CFSpace)
	err = userClient.Get(ctx, client.ObjectKey{Namespace: message.OrgGUID, Name: message.GUID}, cfSpace)
	if err != nil {
		return SpaceRecord{}, fmt.Errorf("failed to get space: %w", apierrors.FromK8sError(err, SpaceResourceType))
	}

	err = k8s.PatchResource(ctx, userClient, cfSpace, func() {
		message.Apply(cfSpace)
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
