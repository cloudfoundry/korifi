package repositories

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"

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

type OrgRecord struct {
	Name        string
	GUID        string
	Suspended   bool
	Labels      map[string]string
	Annotations map[string]string
	CreatedAt   time.Time
	UpdatedAt   time.Time
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
	var orgCR *korifiv1alpha1.CFOrg
	orgCR, err = r.createOrgCR(ctx, info, userClient, &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OrgPrefix + uuid.NewString(),
			Namespace: r.rootNamespace,
		},
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: message.Name,
		},
	})

	if err != nil {
		if webhookError, ok := webhooks.WebhookErrorToValidationError(err); ok { // untested
			return OrgRecord{}, apierrors.NewUnprocessableEntityError(err, webhookError.Error())
		}
		return OrgRecord{}, err
	}

	return OrgRecord{
		Name:        message.Name,
		GUID:        orgCR.Name,
		Suspended:   message.Suspended,
		Labels:      message.Labels,
		Annotations: message.Annotations,
		CreatedAt:   orgCR.CreationTimestamp.Time,
		UpdatedAt:   orgCR.CreationTimestamp.Time,
	}, nil
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
	}

	if !conditionReady {
		return nil, fmt.Errorf("cf org did not get Condition `Ready`: 'True' within timeout period %d ms", r.timeout.Milliseconds())
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

	var records []OrgRecord
	for _, cfOrg := range cfOrgList.Items {
		if !meta.IsStatusConditionTrue(cfOrg.Status.Conditions, StatusConditionReady) {
			continue
		}

		if !matchesFilter(cfOrg.Name, filter.GUIDs) {
			continue
		}

		if !matchesFilter(cfOrg.Spec.DisplayName, filter.Names) {
			continue
		}

		if !authorizedNamespaces[cfOrg.Name] {
			continue
		}

		records = append(records, OrgRecord{
			Name:      cfOrg.Spec.DisplayName,
			GUID:      cfOrg.Name,
			CreatedAt: cfOrg.CreationTimestamp.Time,
			UpdatedAt: cfOrg.CreationTimestamp.Time,
		})
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
