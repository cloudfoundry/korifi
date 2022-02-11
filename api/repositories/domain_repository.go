package repositories

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfdomains,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfdomains/status,verbs=get

type DomainRepo struct {
	privilegedClient  client.Client
	userClientFactory UserK8sClientFactory
}

func NewDomainRepo(
	privilegedClient client.Client,
	userClientFactory UserK8sClientFactory,
) *DomainRepo {
	return &DomainRepo{
		privilegedClient:  privilegedClient,
		userClientFactory: userClientFactory,
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
	domainList := &networkingv1alpha1.CFDomainList{}
	err := r.privilegedClient.List(ctx, domainList, client.MatchingFields{"metadata.name": domainGUID})
	if err != nil {
		return DomainRecord{}, fmt.Errorf("get-domain: privileged list failed: %w", err)
	}

	if len(domainList.Items) == 0 {
		return DomainRecord{}, NewNotFoundError("Domain", err)
	}
	if len(domainList.Items) > 1 {
		return DomainRecord{}, errors.New("get-domain duplicate domains exist")
	}

	domain := cfDomainToDomainRecord(&domainList.Items[0])

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return DomainRecord{}, fmt.Errorf("get-domain failed to build user client: %w", err)
	}

	err = userClient.Get(ctx, client.ObjectKey{Namespace: domain.Namespace, Name: domain.GUID}, &networkingv1alpha1.CFDomain{})
	if k8serrors.IsForbidden(err) {
		return DomainRecord{}, NewForbiddenError(err)
	}

	if err != nil { // untested
		return DomainRecord{}, fmt.Errorf("get-domain user client get failed: %w", err)
	}

	return domain, nil
}

func (r *DomainRepo) ListDomains(ctx context.Context, authInfo authorization.Info, message ListDomainsMessage) ([]DomainRecord, error) {
	cfdomainList := &networkingv1alpha1.CFDomainList{}
	err := r.privilegedClient.List(ctx, cfdomainList)
	if err != nil {
		return []DomainRecord{}, err
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
		return DomainRecord{}, NotFoundError{
			Err:          err,
			ResourceType: "Domain",
		}
	}

	return domainRecords[0], nil
}

func applyDomainListFilterAndOrder(domainList []networkingv1alpha1.CFDomain, message ListDomainsMessage) []networkingv1alpha1.CFDomain {
	var filtered []networkingv1alpha1.CFDomain
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

func returnDomainList(domainList []networkingv1alpha1.CFDomain) []DomainRecord {
	domainRecords := make([]DomainRecord, 0, len(domainList))

	for _, domain := range domainList {
		domainRecords = append(domainRecords, cfDomainToDomainRecord(&domain))
	}
	return domainRecords
}

func cfDomainToDomainRecord(cfDomain *networkingv1alpha1.CFDomain) DomainRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfDomain.ObjectMeta)
	return DomainRecord{
		Name:      cfDomain.Spec.Name,
		GUID:      cfDomain.Name,
		Namespace: cfDomain.Namespace,
		CreatedAt: cfDomain.CreationTimestamp.UTC().Format(TimestampFormat),
		UpdatedAt: updatedAtTime,
	}
}
