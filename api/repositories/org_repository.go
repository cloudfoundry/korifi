package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	OrgPrefix       = "cf-org-"
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

type DeleteOrgMessage struct {
	GUID string
}

type PatchOrgMetadataMessage struct {
	MetadataPatch
	GUID string
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

type OrgRepo struct {
	rootNamespace     string
	privilegedClient  client.WithWatch
	userClientFactory authorization.UserK8sClientFactory
	nsPerms           *authorization.NamespacePermissions
	timeout           time.Duration
}

func NewOrgRepo(
	rootNamespace string,
	privilegedClient client.WithWatch,
	userClientFactory authorization.UserK8sClientFactory,
	nsPerms *authorization.NamespacePermissions,
	timeout time.Duration,
) *OrgRepo {
	return &OrgRepo{
		rootNamespace:     rootNamespace,
		privilegedClient:  privilegedClient,
		userClientFactory: userClientFactory,
		nsPerms:           nsPerms,
		timeout:           timeout,
	}
}

func (r *OrgRepo) CreateOrg(ctx context.Context, info authorization.Info, message CreateOrgMessage) (OrgRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return OrgRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfOrg, err := r.createOrgCR(ctx, info, userClient, &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Name:        OrgPrefix + uuid.NewString(),
			Namespace:   r.rootNamespace,
			Labels:      message.Labels,
			Annotations: message.Annotations,
		},
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: message.Name,
		},
	})
	if err != nil {
		return OrgRecord{}, apierrors.FromK8sError(err, OrgResourceType)
	}

	return cfOrgToOrgRecord(*cfOrg), nil
}

//nolint:dupl
func (r *OrgRepo) createOrgCR(ctx context.Context,
	info authorization.Info,
	userClient client.WithWatch,
	org *korifiv1alpha1.CFOrg,
) (*korifiv1alpha1.CFOrg, error) {
	err := userClient.Create(ctx, org)
	if err != nil {
		return nil, fmt.Errorf("failed to create cf org: %w", apierrors.FromK8sError(err, OrgResourceType))
	}

	timeoutCtx, cancelFn := context.WithTimeout(ctx, r.timeout)
	defer cancelFn()
	watch, err := userClient.Watch(timeoutCtx, &korifiv1alpha1.CFOrgList{},
		client.InNamespace(org.Namespace),
		client.MatchingFields{"metadata.name": org.Name},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to set up watch on cf org: %w", apierrors.FromK8sError(err, OrgResourceType))
	}

	conditionReady := false
	var createdOrg *korifiv1alpha1.CFOrg
	var conditionNotReadyMessage string
	for res := range watch.ResultChan() {
		var ok bool
		createdOrg, ok = res.Object.(*korifiv1alpha1.CFOrg)
		if !ok {
			// should never happen, but avoids panic above
			continue
		}
		if meta.IsStatusConditionTrue(createdOrg.Status.Conditions, StatusConditionReady) {
			watch.Stop()
			conditionReady = true
			break
		}
		readyCondition := meta.FindStatusCondition(createdOrg.Status.Conditions, StatusConditionReady)
		if readyCondition != nil {
			conditionNotReadyMessage = readyCondition.Message
		}
	}

	if !conditionReady {
		err := fmt.Errorf("cf org did not get Condition `Ready`: 'True' within timeout period %d ms", r.timeout.Milliseconds())
		if len(conditionNotReadyMessage) > 0 {
			err = errors.New(conditionNotReadyMessage)
		}

		return nil, apierrors.NewResourceNotReadyError(err)
	}

	return createdOrg, nil
}

func (r *OrgRepo) ListOrgs(ctx context.Context, info authorization.Info, filter ListOrgsMessage) ([]OrgRecord, error) {
	authorizedNamespaces, err := r.nsPerms.GetAuthorizedOrgNamespaces(ctx, info)
	if err != nil {
		return nil, err
	}

	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return []OrgRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfOrgList := new(korifiv1alpha1.CFOrgList)
	err = userClient.List(ctx, cfOrgList, client.InNamespace(r.rootNamespace))
	if err != nil {
		return nil, apierrors.FromK8sError(err, OrgResourceType)
	}

	preds := []func(korifiv1alpha1.CFOrg) bool{
		func(o korifiv1alpha1.CFOrg) bool {
			return authorizedNamespaces[o.Name]
		},
		func(o korifiv1alpha1.CFOrg) bool {
			return meta.IsStatusConditionTrue(o.Status.Conditions, StatusConditionReady)
		},
		SetPredicate(filter.GUIDs, func(s korifiv1alpha1.CFOrg) string { return s.Name }),
		SetPredicate(filter.Names, func(s korifiv1alpha1.CFOrg) string { return s.Spec.DisplayName }),
	}

	var records []OrgRecord
	for _, o := range Filter(cfOrgList.Items, preds...) {
		records = append(records, cfOrgToOrgRecord(o))
	}

	return records, nil
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
	userClient, err := r.userClientFactory.BuildClient(info)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}
	err = userClient.Delete(ctx, &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: r.rootNamespace,
		},
	})

	return apierrors.FromK8sError(err, OrgResourceType)
}

func (r *OrgRepo) PatchOrgMetadata(ctx context.Context, authInfo authorization.Info, message PatchOrgMetadataMessage) (OrgRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return OrgRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfOrg := new(korifiv1alpha1.CFOrg)
	err = userClient.Get(ctx, client.ObjectKey{Namespace: r.rootNamespace, Name: message.GUID}, cfOrg)
	if err != nil {
		return OrgRecord{}, fmt.Errorf("failed to get org: %w", apierrors.FromK8sError(err, OrgResourceType))
	}

	err = k8s.PatchResource(ctx, userClient, cfOrg, func() {
		message.Apply(cfOrg)
	})
	if err != nil {
		return OrgRecord{}, apierrors.FromK8sError(err, OrgResourceType)
	}

	return cfOrgToOrgRecord(*cfOrg), nil
}

func (r *OrgRepo) GetDeletedAt(ctx context.Context, authInfo authorization.Info, orgGUID string) (*time.Time, error) {
	org, err := r.GetOrg(ctx, authInfo, orgGUID)
	if err != nil {
		return nil, err
	}
	return org.DeletedAt, nil
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
