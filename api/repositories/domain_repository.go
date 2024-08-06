package repositories

import (
	"context"
	"fmt"
	"slices"
	"sort"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DomainResourceType = "Domain"
)

type DomainRepo struct {
	userClientFactory  authorization.UserK8sClientFactory
	namespaceRetriever NamespaceRetriever
	rootNamespace      string
}

func NewDomainRepo(
	userClientFactory authorization.UserK8sClientFactory,
	namespaceRetriever NamespaceRetriever,
	rootNamespace string,
) *DomainRepo {
	return &DomainRepo{
		userClientFactory:  userClientFactory,
		namespaceRetriever: namespaceRetriever,
		rootNamespace:      rootNamespace,
	}
}

type DomainRecord struct {
	Name        string
	GUID        string
	Labels      map[string]string
	Annotations map[string]string
	Namespace   string
	CreatedAt   time.Time
	UpdatedAt   *time.Time
	DeletedAt   *time.Time
}

func (r DomainRecord) GetResourceType() string {
	return DomainResourceType
}

type CreateDomainMessage struct {
	Name     string
	Metadata Metadata
}

type UpdateDomainMessage struct {
	GUID          string
	MetadataPatch MetadataPatch
}

type ListDomainsMessage struct {
	Names []string
}

func (m *ListDomainsMessage) matches(d korifiv1alpha1.CFDomain) bool {
	return tools.EmptyOrContains(m.Names, d.Spec.Name)
}

func (r *DomainRepo) GetDomain(ctx context.Context, authInfo authorization.Info, domainGUID string) (DomainRecord, error) {
	ns, err := r.namespaceRetriever.NamespaceFor(ctx, domainGUID, DomainResourceType)
	if err != nil {
		return DomainRecord{}, err
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return DomainRecord{}, fmt.Errorf("get-domain failed to create user client: %w", err)
	}

	domain := &korifiv1alpha1.CFDomain{}
	err = userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: domainGUID}, domain)
	if err != nil {
		return DomainRecord{}, fmt.Errorf("get-domain failed: %w", apierrors.FromK8sError(err, DomainResourceType))
	}

	return cfDomainToDomainRecord(*domain), nil
}

func (r *DomainRepo) CreateDomain(ctx context.Context, authInfo authorization.Info, message CreateDomainMessage) (DomainRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return DomainRecord{}, fmt.Errorf("create-domain failed to create user client: %w", err)
	}

	cfDomain := &korifiv1alpha1.CFDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:        uuid.NewString(),
			Namespace:   r.rootNamespace,
			Labels:      message.Metadata.Labels,
			Annotations: message.Metadata.Annotations,
		},
		Spec: korifiv1alpha1.CFDomainSpec{
			Name: message.Name,
		},
	}

	err = userClient.Create(ctx, cfDomain)
	if err != nil {
		return DomainRecord{}, fmt.Errorf("create-domain failed: %w", apierrors.FromK8sError(err, DomainResourceType))
	}

	return cfDomainToDomainRecord(*cfDomain), nil
}

func (r *DomainRepo) UpdateDomain(ctx context.Context, authInfo authorization.Info, message UpdateDomainMessage) (DomainRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return DomainRecord{}, fmt.Errorf("create-domain failed to create user client: %w", err)
	}

	domain := &korifiv1alpha1.CFDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: r.rootNamespace,
		},
	}

	err = userClient.Get(ctx, client.ObjectKeyFromObject(domain), domain)
	if err != nil {
		return DomainRecord{}, fmt.Errorf("update-domain failed: %w", apierrors.FromK8sError(err, DomainResourceType))
	}

	err = k8s.PatchResource(ctx, userClient, domain, func() {
		message.MetadataPatch.Apply(domain)
	})
	if err != nil {
		return DomainRecord{}, fmt.Errorf("failed to patch domain metadata: %w", apierrors.FromK8sError(err, DomainResourceType))
	}

	return cfDomainToDomainRecord(*domain), nil
}

func (r *DomainRepo) ListDomains(ctx context.Context, authInfo authorization.Info, message ListDomainsMessage) ([]DomainRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []DomainRecord{}, fmt.Errorf("list-domain failed to create user client: %w", err)
	}

	cfdomainList := &korifiv1alpha1.CFDomainList{}
	err = userClient.List(ctx, cfdomainList, client.InNamespace(r.rootNamespace))
	if err != nil {
		if k8serrors.IsForbidden(err) {
			return []DomainRecord{}, nil
		}
		return []DomainRecord{}, fmt.Errorf("failed to list domains in namespace %s: %w", r.rootNamespace, apierrors.FromK8sError(err, DomainResourceType))
	}

	domainRecords := slices.Collect(it.Map(
		itx.FromSlice(cfdomainList.Items).Filter(message.matches),
		cfDomainToDomainRecord,
	))
	sort.Slice(domainRecords, func(i, j int) bool {
		return domainRecords[i].CreatedAt.Before(domainRecords[j].CreatedAt)
	})

	return domainRecords, nil
}

func (r *DomainRepo) DeleteDomain(ctx context.Context, authInfo authorization.Info, domainGUID string) error {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("delete-domain failed to create user client: %w", err)
	}

	cfDomain := &korifiv1alpha1.CFDomain{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      domainGUID,
		},
	}

	err = userClient.Delete(ctx, cfDomain)
	if err != nil {
		return apierrors.FromK8sError(err, DomainResourceType)
	}

	return nil
}

func (r *DomainRepo) GetDeletedAt(ctx context.Context, authInfo authorization.Info, domainGUID string) (*time.Time, error) {
	domain, err := r.GetDomain(ctx, authInfo, domainGUID)
	return domain.DeletedAt, err
}

func cfDomainToDomainRecord(cfDomain korifiv1alpha1.CFDomain) DomainRecord {
	return DomainRecord{
		Name:        cfDomain.Spec.Name,
		GUID:        cfDomain.Name,
		Namespace:   cfDomain.Namespace,
		CreatedAt:   cfDomain.CreationTimestamp.Time,
		UpdatedAt:   getLastUpdatedTime(&cfDomain),
		DeletedAt:   golangTime(cfDomain.DeletionTimestamp),
		Labels:      cfDomain.Labels,
		Annotations: cfDomain.Annotations,
	}
}
