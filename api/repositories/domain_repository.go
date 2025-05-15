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
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/google/uuid"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DomainResourceType = "Domain"
)

type DomainRepo struct {
	klient        Klient
	rootNamespace string
}

func NewDomainRepo(
	klient Klient,
	rootNamespace string,
) *DomainRepo {
	return &DomainRepo{
		klient:        klient,
		rootNamespace: rootNamespace,
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

func (m *ListDomainsMessage) toListOptions(rootNamespace string) []ListOption {
	return []ListOption{
		WithLabelIn(korifiv1alpha1.CFEncodedDomainNameLabelKey, tools.EncodeValuesToSha224(m.Names...)),
		InNamespace(rootNamespace),
	}
}

func (r *DomainRepo) GetDomain(ctx context.Context, authInfo authorization.Info, domainGUID string) (DomainRecord, error) {
	domain := &korifiv1alpha1.CFDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name: domainGUID,
		},
	}
	err := r.klient.Get(ctx, domain)
	if err != nil {
		return DomainRecord{}, fmt.Errorf("get-domain failed: %w", apierrors.FromK8sError(err, DomainResourceType))
	}

	return cfDomainToDomainRecord(*domain), nil
}

func (r *DomainRepo) CreateDomain(ctx context.Context, authInfo authorization.Info, message CreateDomainMessage) (DomainRecord, error) {
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

	err := r.klient.Create(ctx, cfDomain)
	if err != nil {
		return DomainRecord{}, fmt.Errorf("create-domain failed: %w", apierrors.FromK8sError(err, DomainResourceType))
	}

	return cfDomainToDomainRecord(*cfDomain), nil
}

func (r *DomainRepo) UpdateDomain(ctx context.Context, authInfo authorization.Info, message UpdateDomainMessage) (DomainRecord, error) {
	domain := &korifiv1alpha1.CFDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: r.rootNamespace,
		},
	}

	err := r.klient.Get(ctx, domain)
	if err != nil {
		return DomainRecord{}, fmt.Errorf("update-domain failed: %w", apierrors.FromK8sError(err, DomainResourceType))
	}

	err = r.klient.Patch(ctx, domain, func() error {
		message.MetadataPatch.Apply(domain)
		return nil
	})
	if err != nil {
		return DomainRecord{}, fmt.Errorf("failed to patch domain metadata: %w", apierrors.FromK8sError(err, DomainResourceType))
	}

	return cfDomainToDomainRecord(*domain), nil
}

func (r *DomainRepo) ListDomains(ctx context.Context, authInfo authorization.Info, message ListDomainsMessage) ([]DomainRecord, error) {
	cfdomainList := &korifiv1alpha1.CFDomainList{}
	err := r.klient.List(ctx, cfdomainList, message.toListOptions(r.rootNamespace)...)
	if err != nil {
		if k8serrors.IsForbidden(err) {
			return []DomainRecord{}, nil
		}
		return []DomainRecord{}, fmt.Errorf("failed to list domains in namespace %s: %w", r.rootNamespace, apierrors.FromK8sError(err, DomainResourceType))
	}

	domainRecords := slices.Collect(it.Map(slices.Values(cfdomainList.Items), cfDomainToDomainRecord))
	sort.Slice(domainRecords, func(i, j int) bool {
		return domainRecords[i].CreatedAt.Before(domainRecords[j].CreatedAt)
	})

	return domainRecords, nil
}

func (r *DomainRepo) DeleteDomain(ctx context.Context, authInfo authorization.Info, domainGUID string) error {
	cfDomain := &korifiv1alpha1.CFDomain{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      domainGUID,
		},
	}

	err := r.klient.Delete(ctx, cfDomain)
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
