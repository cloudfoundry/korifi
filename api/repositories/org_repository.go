package repositories

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

//+kubebuilder:rbac:groups=hnc.x-k8s.io,resources=subnamespaceanchors,verbs=list;create;watch
//+kubebuilder:rbac:groups="",resources=serviceaccounts;secrets,verbs=get;list;create;delete;watch

//counterfeiter:generate -o fake -fake-name CFOrgRepository . CFOrgRepository
//counterfeiter:generate -o fake -fake-name CFSpaceRepository . CFSpaceRepository

type CFOrgRepository interface {
	CreateOrg(context context.Context, org CreateOrgMessage) (OrgRecord, error)
	ListOrgs(context context.Context, orgNames []string) ([]OrgRecord, error)
}

type CFSpaceRepository interface {
	CreateSpace(context.Context, CreateSpaceMessage) (SpaceRecord, error)
	ListSpaces(context.Context, []string, []string) ([]SpaceRecord, error)
}

const (
	OrgNameLabel   = "cloudfoundry.org/org-name"
	SpaceNameLabel = "cloudfoundry.org/space-name"
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
	rootNamespace    string
	privilegedClient client.WithWatch
	timeout          time.Duration
}

func NewOrgRepo(rootNamespace string, privilegedClient client.WithWatch, timeout time.Duration) *OrgRepo {
	return &OrgRepo{
		rootNamespace:    rootNamespace,
		privilegedClient: privilegedClient,
		timeout:          timeout,
	}
}

func (r *OrgRepo) CreateOrg(ctx context.Context, org CreateOrgMessage) (OrgRecord, error) {
	anchor, err := r.createSubnamespaceAnchor(ctx, &v1alpha2.SubnamespaceAnchor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      org.GUID,
			Namespace: r.rootNamespace,
			Labels: map[string]string{
				OrgNameLabel: org.Name,
			},
		},
	})
	if err != nil {
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

func (r *OrgRepo) CreateSpace(ctx context.Context, message CreateSpaceMessage) (SpaceRecord, error) {
	anchor, err := r.createSubnamespaceAnchor(ctx, &v1alpha2.SubnamespaceAnchor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: message.OrganizationGUID,
			Labels: map[string]string{
				SpaceNameLabel: message.Name,
			},
		},
	})
	if err != nil {
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

func (r *OrgRepo) createSubnamespaceAnchor(ctx context.Context, anchor *v1alpha2.SubnamespaceAnchor) (*v1alpha2.SubnamespaceAnchor, error) {
	err := r.privilegedClient.Create(ctx, anchor)
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

	return createdAnchor, nil
}

func (r *OrgRepo) ListOrgs(ctx context.Context, names []string) ([]OrgRecord, error) {
	subnamespaceAnchorList := &v1alpha2.SubnamespaceAnchorList{}

	options := []client.ListOption{client.InNamespace(r.rootNamespace)}
	if len(names) > 0 {
		namesRequirement, err := labels.NewRequirement(OrgNameLabel, selection.In, names)
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

	records := []OrgRecord{}
	for _, anchor := range subnamespaceAnchorList.Items {
		if anchor.Status.State != v1alpha2.Ok {
			continue
		}

		records = append(records, OrgRecord{
			Name:      anchor.Labels[OrgNameLabel],
			GUID:      anchor.Name,
			CreatedAt: anchor.CreationTimestamp.Time,
			UpdatedAt: anchor.CreationTimestamp.Time,
		})
	}

	return records, nil
}

func (r *OrgRepo) ListSpaces(ctx context.Context, organizationGUIDs, names []string) ([]SpaceRecord, error) {
	subnamespaceAnchorList := &v1alpha2.SubnamespaceAnchorList{}

	err := r.privilegedClient.List(ctx, subnamespaceAnchorList)
	if err != nil {
		return nil, err
	}

	orgsFilter := toMap(organizationGUIDs)
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

	nameFilter := toMap(names)
	records := []SpaceRecord{}
	for _, anchor := range subnamespaceAnchorList.Items {
		if anchor.Status.State != v1alpha2.Ok {
			continue
		}

		spaceName := anchor.Labels[SpaceNameLabel]
		if !matchFilter(nameFilter, spaceName) {
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

	return records, nil
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
