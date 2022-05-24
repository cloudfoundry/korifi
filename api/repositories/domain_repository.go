package repositories

import (
	"context"
	"fmt"
	"sort"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
	CreatedAt   string
	UpdatedAt   string
}

type ListDomainsMessage struct {
	Names []string
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

	domain := &v1alpha1.CFDomain{}
	err = userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: domainGUID}, domain)
	if err != nil {
		return DomainRecord{}, apierrors.NewForbiddenError(err, DomainResourceType)
	}

	return cfDomainToDomainRecord(domain), nil
}

func (r *DomainRepo) ListDomains(ctx context.Context, authInfo authorization.Info, message ListDomainsMessage) ([]DomainRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []DomainRecord{}, fmt.Errorf("list-domain failed to create user client: %w", err)
	}

	cfdomainList := &v1alpha1.CFDomainList{}
	err = userClient.List(ctx, cfdomainList, client.InNamespace(r.rootNamespace))
	if err != nil {
		if k8serrors.IsForbidden(err) {
			return []DomainRecord{}, nil
		}
		// untested
		return []DomainRecord{}, fmt.Errorf("failed to list domains in namespace %s: %w", r.rootNamespace, apierrors.FromK8sError(err, DomainResourceType))
	}

	filtered := applyDomainListFilterAndOrder(cfdomainList.Items, message)

	return returnDomainList(filtered), nil
}

func (r *DomainRepo) GetDomainByName(ctx context.Context, authInfo authorization.Info, domainName string) (DomainRecord, error) {
	domainRecords, err := r.ListDomains(ctx, authInfo, ListDomainsMessage{
		Names: []string{domainName},
	})
	if err != nil {
		return DomainRecord{}, err
	}

	if len(domainRecords) == 0 {
		return DomainRecord{}, apierrors.NewNotFoundError(fmt.Errorf("domain %q not found", domainName), DomainResourceType)
	}

	return domainRecords[0], nil
}

func applyDomainListFilterAndOrder(domainList []v1alpha1.CFDomain, message ListDomainsMessage) []v1alpha1.CFDomain {
	var filtered []v1alpha1.CFDomain
	if len(message.Names) > 0 {
		for _, domain := range domainList {
			for _, name := range message.Names {
				if domain.Spec.Name == name {
					filtered = append(filtered, domain)
				}
			}
		}
	} else {
		filtered = domainList
	}

	// TODO: use the future message.Order fields to reorder the list of results
	// For now, we order by created_at by default- if you really want to optimize runtime you can use bucketsort
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreationTimestamp.Before(&filtered[j].CreationTimestamp)
	})

	return filtered
}

func returnDomainList(domainList []v1alpha1.CFDomain) []DomainRecord {
	domainRecords := make([]DomainRecord, 0, len(domainList))

	for i := range domainList {
		domainRecords = append(domainRecords, cfDomainToDomainRecord(&domainList[i]))
	}
	return domainRecords
}

func cfDomainToDomainRecord(cfDomain *v1alpha1.CFDomain) DomainRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfDomain.ObjectMeta)
	return DomainRecord{
		Name:      cfDomain.Spec.Name,
		GUID:      cfDomain.Name,
		Namespace: cfDomain.Namespace,
		CreatedAt: cfDomain.CreationTimestamp.UTC().Format(TimestampFormat),
		UpdatedAt: updatedAtTime,
	}
}
