package repositories

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/controllers/apis/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"

	"github.com/google/uuid"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=hnc.x-k8s.io,resources=subnamespaceanchors,verbs=list;watch
//+kubebuilder:rbac:groups=hnc.x-k8s.io,resources=hierarchyconfigurations,verbs=get

const (
	SpaceNameLabel    = "cloudfoundry.org/space-name"
	SpacePrefix       = "cf-space-"
	SpaceResourceType = "Space"
)

type CreateSpaceMessage struct {
	Name                     string
	OrganizationGUID         string
	ImageRegistryCredentials string
}

type ListSpacesMessage struct {
	Names             []string
	GUIDs             []string
	OrganizationGUIDs []string
}

type DeleteSpaceMessage struct {
	GUID             string
	OrganizationGUID string
}

type SpaceRecord struct {
	Name             string
	GUID             string
	OrganizationGUID string
	Labels           map[string]string
	Annotations      map[string]string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type SpaceRepo struct {
	orgRepo            *OrgRepo
	namespaceRetriever NamespaceRetriever
	userClientFactory  authorization.UserK8sClientFactory
	nsPerms            *authorization.NamespacePermissions
	timeout            time.Duration
}

func NewSpaceRepo(
	namespaceRetriever NamespaceRetriever,
	orgRepo *OrgRepo,
	userClientFactory authorization.UserK8sClientFactory,
	nsPerms *authorization.NamespacePermissions,
	timeout time.Duration,
) *SpaceRepo {
	return &SpaceRepo{
		orgRepo:            orgRepo,
		namespaceRetriever: namespaceRetriever,
		userClientFactory:  userClientFactory,
		nsPerms:            nsPerms,
		timeout:            timeout,
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

	spaceCR, err := r.createSpaceCR(ctx, info, userClient, &v1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SpacePrefix + uuid.NewString(),
			Namespace: message.OrganizationGUID,
		},
		Spec: v1alpha1.CFSpaceSpec{
			DisplayName: message.Name,
		},
	})
	if err != nil {
		if webhookError, ok := webhooks.WebhookErrorToValidationError(err); ok {
			return SpaceRecord{}, apierrors.NewUnprocessableEntityError(err, webhookError.Error())
		}
		return SpaceRecord{}, err
	}

	return SpaceRecord{
		Name:             message.Name,
		GUID:             spaceCR.Name,
		OrganizationGUID: message.OrganizationGUID,
		CreatedAt:        spaceCR.CreationTimestamp.Time,
		UpdatedAt:        spaceCR.CreationTimestamp.Time,
	}, nil
}

//nolint:dupl
func (r *SpaceRepo) createSpaceCR(ctx context.Context,
	info authorization.Info,
	userClient client.WithWatch,
	space *v1alpha1.CFSpace,
) (*v1alpha1.CFSpace, error) {
	err := userClient.Create(ctx, space)
	if err != nil {
		return nil, fmt.Errorf("failed to create cf space: %w", apierrors.FromK8sError(err, SpaceResourceType))
	}

	timeoutCtx, cancelFn := context.WithTimeout(ctx, r.timeout)
	defer cancelFn()
	watch, err := userClient.Watch(timeoutCtx, &v1alpha1.CFSpaceList{},
		client.InNamespace(space.Namespace),
		client.MatchingFields{"metadata.name": space.Name},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to set up watch on cf space: %w", apierrors.FromK8sError(err, SpaceResourceType))
	}

	conditionReady := false
	var createdSpace *v1alpha1.CFSpace
	for res := range watch.ResultChan() {
		var ok bool
		createdSpace, ok = res.Object.(*v1alpha1.CFSpace)
		if !ok {
			// should never happen, but avoids panic above
			continue
		}
		if meta.IsStatusConditionTrue(createdSpace.Status.Conditions, StatusConditionReady) {
			watch.Stop()
			conditionReady = true
			break
		}
	}

	if !conditionReady {
		return nil, fmt.Errorf("cf space did not get Condition `Ready`: 'True' within timeout period %d ms", r.timeout.Milliseconds())
	}

	// wait for the namespace to be created and user to have permissions
	timeoutChan := time.After(r.timeout)

	t1 := time.Now()
outer:
	for {
		select {
		case <-timeoutChan:
			// HNC is broken
			return nil, fmt.Errorf("failed establishing permissions in new namespace after %s: %w", time.Since(t1), err)
		default:
			var authorizedNamespaces map[string]bool
			authorizedNamespaces, err = r.nsPerms.GetAuthorizedSpaceNamespaces(ctx, info)
			if err != nil {
				return nil, err
			}

			if _, ok := authorizedNamespaces[space.Name]; ok {
				break outer
			}

			time.Sleep(500 * time.Millisecond)
		}
	}

	return createdSpace, nil
}

func (r *SpaceRepo) ListSpaces(ctx context.Context, info authorization.Info, message ListSpacesMessage) ([]SpaceRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return []SpaceRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	authorizedOrgNamespaces, err := r.nsPerms.GetAuthorizedOrgNamespaces(ctx, info)
	if err != nil {
		return nil, err
	}

	cfSpaces := []v1alpha1.CFSpace{}

	for org := range authorizedOrgNamespaces {
		cfSpaceList := new(v1alpha1.CFSpaceList)

		err = userClient.List(ctx, cfSpaceList, client.InNamespace(org))
		if k8serrors.IsForbidden(err) {
			continue
		}
		if err != nil {
			return nil, apierrors.FromK8sError(err, SpaceResourceType)
		}

		cfSpaces = append(cfSpaces, cfSpaceList.Items...)
	}

	authorizedSpaceNamespaces, err := r.nsPerms.GetAuthorizedSpaceNamespaces(ctx, info)
	if err != nil {
		return nil, err
	}

	var records []SpaceRecord
	for _, cfSpace := range cfSpaces {
		if !meta.IsStatusConditionTrue(cfSpace.Status.Conditions, StatusConditionReady) {
			continue
		}

		if !matchesFilter(cfSpace.Namespace, message.OrganizationGUIDs) {
			continue
		}

		if !matchesFilter(cfSpace.Name, message.GUIDs) {
			continue
		}

		if !matchesFilter(cfSpace.Spec.DisplayName, message.Names) {
			continue
		}

		if !authorizedSpaceNamespaces[cfSpace.Name] {
			continue
		}

		records = append(records, cfSpaceToSpaceRecord(cfSpace))
	}

	return records, nil
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

	cfSpace := v1alpha1.CFSpace{}
	err = userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: spaceGUID}, &cfSpace)
	if err != nil {
		return SpaceRecord{}, fmt.Errorf("failed to get space: %w", apierrors.FromK8sError(err, SpaceResourceType))
	}

	return cfSpaceToSpaceRecord(cfSpace), nil
}

func cfSpaceToSpaceRecord(cfSpace v1alpha1.CFSpace) SpaceRecord {
	return SpaceRecord{
		Name:             cfSpace.Spec.DisplayName,
		GUID:             cfSpace.Name,
		OrganizationGUID: cfSpace.Namespace,
		CreatedAt:        cfSpace.CreationTimestamp.Time,
		UpdatedAt:        cfSpace.CreationTimestamp.Time,
	}
}

func (r *SpaceRepo) DeleteSpace(ctx context.Context, info authorization.Info, message DeleteSpaceMessage) error {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	err = userClient.Delete(ctx, &v1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: message.OrganizationGUID,
		},
	})

	return apierrors.FromK8sError(err, SpaceResourceType)
}
