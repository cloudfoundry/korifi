package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

//+kubebuilder:rbac:groups=hnc.x-k8s.io,resources=subnamespaceanchors,verbs=list;create;delete;watch
//+kubebuilder:rbac:groups=hnc.x-k8s.io,resources=hierarchyconfigurations,verbs=get;list;watch;update
//+kubebuilder:rbac:groups="",resources=serviceaccounts;secrets,verbs=get;list;create;delete;watch

//counterfeiter:generate -o fake -fake-name CFSpaceRepository . CFSpaceRepository

type CFSpaceRepository interface {
	CreateSpace(context.Context, authorization.Info, CreateSpaceMessage) (SpaceRecord, error)
	ListSpaces(context.Context, authorization.Info, ListSpacesMessage) ([]SpaceRecord, error)
	GetSpace(context.Context, authorization.Info, string) (SpaceRecord, error)
	DeleteSpace(context.Context, authorization.Info, DeleteSpaceMessage) error
}

const (
	OrgNameLabel          = "cloudfoundry.org/org-name"
	SpaceNameLabel        = "cloudfoundry.org/space-name"
	hierarchyMetadataName = "hierarchy"

	OrgResourceType   = "Org"
	SpaceResourceType = "Space"
)

type CreateOrgMessage struct {
	Name        string
	GUID        string
	Suspended   bool
	Labels      map[string]string
	Annotations map[string]string
}

type CreateSpaceMessage struct {
	Name                     string
	GUID                     string
	OrganizationGUID         string
	ImageRegistryCredentials string
}

type ListOrgsMessage struct {
	Names []string
	GUIDs []string
}

type ListSpacesMessage struct {
	Names             []string
	GUIDs             []string
	OrganizationGUIDs []string
}

type DeleteOrgMessage struct {
	GUID string
}

type DeleteSpaceMessage struct {
	GUID             string
	OrganizationGUID string
}

type OrgRecord struct {
	Name        string
	GUID        string
	Suspended   bool
	Labels      map[string]string
	Annotations map[string]string
	CreatedAt   time.Time
	UpdatedAt   time.Time
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

type OrgRepo struct {
	rootNamespace     string
	privilegedClient  client.WithWatch
	userClientFactory UserK8sClientFactory
	nsPerms           *authorization.NamespacePermissions
	timeout           time.Duration
	authEnabled       bool
}

func NewOrgRepo(
	rootNamespace string,
	privilegedClient client.WithWatch,
	userClientFactory UserK8sClientFactory,
	nsPerms *authorization.NamespacePermissions,
	timeout time.Duration,
	authEnabled bool,
) *OrgRepo {
	return &OrgRepo{
		rootNamespace:     rootNamespace,
		privilegedClient:  privilegedClient,
		userClientFactory: userClientFactory,
		nsPerms:           nsPerms,
		timeout:           timeout,
		authEnabled:       authEnabled,
	}
}

func (r *OrgRepo) CreateOrg(ctx context.Context, info authorization.Info, org CreateOrgMessage) (OrgRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return OrgRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}
	anchor, err := r.createSubnamespaceAnchor(ctx, userClient, &v1alpha2.SubnamespaceAnchor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      org.GUID,
			Namespace: r.rootNamespace,
			Labels: map[string]string{
				OrgNameLabel: org.Name,
			},
		},
	})
	if err != nil {
		if apierrors.IsForbidden(err) {
			return OrgRecord{}, NewForbiddenError(OrgResourceType, err)
		}

		return OrgRecord{}, err
	}

	orgRecord := OrgRecord{
		Name:        org.Name,
		GUID:        anchor.Name,
		Suspended:   org.Suspended,
		Labels:      org.Labels,
		Annotations: org.Annotations,
		CreatedAt:   anchor.CreationTimestamp.Time,
		UpdatedAt:   anchor.CreationTimestamp.Time,
	}

	return orgRecord, nil
}

func (r *OrgRepo) CreateSpace(ctx context.Context, info authorization.Info, message CreateSpaceMessage) (SpaceRecord, error) {
	_, err := r.GetOrg(ctx, info, message.OrganizationGUID)
	if errors.As(err, &NotFoundError{}) {
		return SpaceRecord{}, err
	}
	if err != nil {
		return SpaceRecord{}, fmt.Errorf("failed to get parent organization: %w", err)
	}

	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return SpaceRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	anchor, err := r.createSubnamespaceAnchor(ctx, userClient, &v1alpha2.SubnamespaceAnchor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: message.OrganizationGUID,
			Labels: map[string]string{
				SpaceNameLabel: message.Name,
			},
		},
	})
	if err != nil {
		if apierrors.IsForbidden(err) {
			return SpaceRecord{}, NewForbiddenError(SpaceResourceType, err)
		}

		return SpaceRecord{}, err
	}

	kpackServiceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kpack-service-account",
			Namespace: message.GUID,
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: message.ImageRegistryCredentials},
		},
		Secrets: []corev1.ObjectReference{
			{Name: message.ImageRegistryCredentials},
		},
	}
	err = r.privilegedClient.Create(ctx, &kpackServiceAccount)
	if err != nil {
		return SpaceRecord{}, err
	}

	eiriniServiceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "eirini",
			Namespace: message.GUID,
		},
	}
	err = r.privilegedClient.Create(ctx, &eiriniServiceAccount)
	if err != nil {
		return SpaceRecord{}, err
	}

	record := SpaceRecord{
		Name:             message.Name,
		GUID:             anchor.Name,
		OrganizationGUID: message.OrganizationGUID,
		CreatedAt:        anchor.CreationTimestamp.Time,
		UpdatedAt:        anchor.CreationTimestamp.Time,
	}

	return record, nil
}

func (r *OrgRepo) createSubnamespaceAnchor(ctx context.Context, userClient client.Client, anchor *v1alpha2.SubnamespaceAnchor) (*v1alpha2.SubnamespaceAnchor, error) {
	err := userClient.Create(ctx, anchor)
	if err != nil {
		return nil, fmt.Errorf("failed to create subnamespaceanchor: %w", err)
	}

	timeoutCtx, cancelFn := context.WithTimeout(ctx, r.timeout)
	defer cancelFn()

	watch, err := r.privilegedClient.Watch(timeoutCtx, &v1alpha2.SubnamespaceAnchorList{},
		client.InNamespace(anchor.Namespace),
		client.MatchingFields{"metadata.name": anchor.Name},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to set up watch on subnamespaceanchors: %w", err)
	}

	stateOK := false
	var createdAnchor *v1alpha2.SubnamespaceAnchor
	for res := range watch.ResultChan() {
		var ok bool
		createdAnchor, ok = res.Object.(*v1alpha2.SubnamespaceAnchor)
		if !ok {
			// should never happen, but avoids panic above
			continue
		}
		if createdAnchor.Status.State == v1alpha2.Ok {
			watch.Stop()
			stateOK = true
			break
		}
	}

	if !stateOK {
		return nil, fmt.Errorf("subnamespaceanchor did not get state 'ok' within timeout period %d ms", r.timeout.Milliseconds())
	}

	// wait for the user to have permissions in the new namespace
	const attempts = 9
	for i := 0; i < attempts; i++ {
		appList := &workloadsv1alpha1.CFAppList{}
		err := userClient.List(ctx, appList, client.InNamespace(anchor.Name))
		if err == nil {
			break
		}
		if !apierrors.IsForbidden(err) {
			return nil, err
		}
		if i < attempts-1 {
			time.Sleep(50 * time.Millisecond * (1 << i))
		} else {
			// HNC is broken
			return nil, errors.New("failed establishing permissions in new namespace")
		}
	}

	return createdAnchor, nil
}

func (r *OrgRepo) ListOrgs(ctx context.Context, info authorization.Info, filter ListOrgsMessage) ([]OrgRecord, error) {
	subnamespaceAnchorList := &v1alpha2.SubnamespaceAnchorList{}

	options := []client.ListOption{client.InNamespace(r.rootNamespace)}
	if len(filter.Names) > 0 {
		namesRequirement, err := labels.NewRequirement(OrgNameLabel, selection.In, filter.Names)
		if err != nil {
			return nil, err
		}
		namesSelector := client.MatchingLabelsSelector{
			Selector: labels.NewSelector().Add(*namesRequirement),
		}
		options = append(options, namesSelector)
	}

	err := r.privilegedClient.List(ctx, subnamespaceAnchorList, options...)
	if err != nil {
		return nil, err
	}

	guidFilter := toMap(filter.GUIDs)
	records := []OrgRecord{}
	for _, anchor := range subnamespaceAnchorList.Items {
		if anchor.Status.State != v1alpha2.Ok {
			continue
		}

		if !matchFilter(guidFilter, anchor.Name) {
			continue
		}

		records = append(records, OrgRecord{
			Name:      anchor.Labels[OrgNameLabel],
			GUID:      anchor.Name,
			CreatedAt: anchor.CreationTimestamp.Time,
			UpdatedAt: anchor.CreationTimestamp.Time,
		})
	}

	if !r.authEnabled {
		return records, nil
	}

	authorizedNamespaces, err := r.nsPerms.GetAuthorizedOrgNamespaces(ctx, info)
	if err != nil {
		return nil, err
	}

	result := []OrgRecord{}
	for _, org := range records {
		if !authorizedNamespaces[org.GUID] {
			continue
		}

		result = append(result, org)
	}

	return result, nil
}

func (r *OrgRepo) ListSpaces(ctx context.Context, info authorization.Info, message ListSpacesMessage) ([]SpaceRecord, error) {
	subnamespaceAnchorList := &v1alpha2.SubnamespaceAnchorList{}

	err := r.privilegedClient.List(ctx, subnamespaceAnchorList)
	if err != nil {
		return nil, err
	}

	orgsFilter := toMap(message.OrganizationGUIDs)
	orgUIDs := map[string]struct{}{}
	for _, anchor := range subnamespaceAnchorList.Items {
		if anchor.Namespace != r.rootNamespace {
			continue
		}

		if !matchFilter(orgsFilter, anchor.Name) {
			continue
		}

		orgUIDs[anchor.Name] = struct{}{}
	}

	nameFilter := toMap(message.Names)
	guidFilter := toMap(message.GUIDs)
	records := []SpaceRecord{}
	for _, anchor := range subnamespaceAnchorList.Items {
		if anchor.Status.State != v1alpha2.Ok {
			continue
		}

		spaceName := anchor.Labels[SpaceNameLabel]
		if !matchFilter(nameFilter, spaceName) {
			continue
		}

		if !matchFilter(guidFilter, anchor.Name) {
			continue
		}

		if _, ok := orgUIDs[anchor.Namespace]; !ok {
			continue
		}

		records = append(records, SpaceRecord{
			Name:             spaceName,
			GUID:             anchor.Name,
			OrganizationGUID: anchor.Namespace,
			CreatedAt:        anchor.CreationTimestamp.Time,
			UpdatedAt:        anchor.CreationTimestamp.Time,
		})
	}

	if !r.authEnabled {
		return records, nil
	}

	authorizedNamespaces, err := r.nsPerms.GetAuthorizedSpaceNamespaces(ctx, info)
	if err != nil {
		return nil, err
	}

	result := []SpaceRecord{}
	for _, space := range records {
		if !authorizedNamespaces[space.GUID] {
			continue
		}

		result = append(result, space)
	}

	return result, nil
}

func matchFilter(filter map[string]struct{}, value string) bool {
	if len(filter) == 0 {
		return true
	}

	_, ok := filter[value]
	return ok
}

func toMap(elements []string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, element := range elements {
		result[element] = struct{}{}
	}

	return result
}

func (r *OrgRepo) GetSpace(ctx context.Context, info authorization.Info, spaceGUID string) (SpaceRecord, error) {
	spaceRecords, err := r.ListSpaces(ctx, info, ListSpacesMessage{GUIDs: []string{spaceGUID}})
	if err != nil {
		return SpaceRecord{}, err
	}

	if len(spaceRecords) == 0 {
		return SpaceRecord{}, NewNotFoundError(SpaceResourceType, nil)
	}

	return spaceRecords[0], nil
}

func (r *OrgRepo) GetOrg(ctx context.Context, info authorization.Info, orgGUID string) (OrgRecord, error) {
	orgRecords, err := r.ListOrgs(ctx, info, ListOrgsMessage{GUIDs: []string{orgGUID}})
	if err != nil {
		return OrgRecord{}, err
	}

	if len(orgRecords) == 0 {
		return OrgRecord{}, NewNotFoundError(OrgResourceType, err)
	}

	return orgRecords[0], nil
}

func (r *OrgRepo) DeleteOrg(ctx context.Context, info authorization.Info, message DeleteOrgMessage) error {
	var err error
	hierarchyObj := v1alpha2.HierarchyConfiguration{}
	err = r.privilegedClient.Get(ctx, types.NamespacedName{Name: hierarchyMetadataName, Namespace: message.GUID}, &hierarchyObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return NewNotFoundError(OrgResourceType, err)
		}
		return err
	}

	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	hierarchyObj.Spec.AllowCascadingDeletion = true
	err = userClient.Update(ctx, &hierarchyObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return NewNotFoundError(OrgResourceType, err)
		}
		if apierrors.IsForbidden(err) {
			return NewForbiddenError(OrgResourceType, err)
		}
		return err
	}

	err = userClient.Delete(ctx, &v1alpha2.SubnamespaceAnchor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: r.rootNamespace,
		},
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return NewNotFoundError(OrgResourceType, err)
		}
		if apierrors.IsForbidden(err) {
			return NewForbiddenError(OrgResourceType, err)
		}
		return err
	}

	return nil
}

func (r *OrgRepo) DeleteSpace(ctx context.Context, info authorization.Info, message DeleteSpaceMessage) error {
	if r.authEnabled {
		userClient, err := r.userClientFactory.BuildClient(info)
		if err != nil {
			return fmt.Errorf("failed to build user client: %w", err)
		}
		err = userClient.Delete(ctx, &v1alpha2.SubnamespaceAnchor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      message.GUID,
				Namespace: message.OrganizationGUID,
			},
		})
		if err == nil {
			return nil
		}

		if apierrors.IsForbidden(err) {
			return NewForbiddenError(SpaceResourceType, err)
		}

		return err
	}

	return r.privilegedClient.Delete(ctx, &v1alpha2.SubnamespaceAnchor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: message.OrganizationGUID,
		},
	})
}
