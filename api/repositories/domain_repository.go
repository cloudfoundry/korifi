package repositories

import (
	"context"
	"sort"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfdomains,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfdomains/status,verbs=get

type DomainRepo struct {
	privilegedClient client.Client
}

func NewDomainRepo(privilegedClient client.Client) *DomainRepo {
	return &DomainRepo{privilegedClient: privilegedClient}
}

type DomainRecord struct {
	Name        string
	GUID        string
	Labels      map[string]string
	Annotations map[string]string
	CreatedAt   string
	UpdatedAt   string
}

type DomainListMessage struct {
	Names []string
}

func (r *DomainRepo) FetchDomain(ctx context.Context, authInfo authorization.Info, domainGUID string) (DomainRecord, error) {
	domain := &networkingv1alpha1.CFDomain{}
	err := r.privilegedClient.Get(ctx, types.NamespacedName{Name: domainGUID}, domain)
	if err != nil {
		switch errtype := err.(type) {
		case *k8serrors.StatusError:
			reason := errtype.Status().Reason
			if reason == metav1.StatusReasonNotFound || reason == metav1.StatusReasonUnauthorized {
				return DomainRecord{}, PermissionDeniedOrNotFoundError{Err: err}
			}
		}

		return DomainRecord{}, err
	}

	return cfDomainToDomainRecord(domain), nil
}

func (r *DomainRepo) FetchDomainList(ctx context.Context, authInfo authorization.Info, message DomainListMessage) ([]DomainRecord, error) {
	cfdomainList := &networkingv1alpha1.CFDomainList{}
	err := r.privilegedClient.List(ctx, cfdomainList)
	if err != nil {
		return []DomainRecord{}, err
	}

	filtered := r.applyDomainListFilterAndOrder(cfdomainList.Items, message)

	return r.returnDomainList(filtered), nil
}

func (r *DomainRepo) FetchDomainByName(ctx context.Context, authInfo authorization.Info, domainName string) (DomainRecord, error) {
	domainRecords, err := r.FetchDomainList(ctx, authInfo, DomainListMessage{
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

func (r *DomainRepo) FetchDefaultDomain(ctx context.Context, authInfo authorization.Info) (DomainRecord, error) {
	domainList, err := r.FetchDomainList(ctx, authInfo, DomainListMessage{})
	if err != nil { // untested
		return DomainRecord{}, err
	}
	if len(domainList) == 0 {
		return DomainRecord{}, NotFoundError{ResourceType: "Domain"}
	}
	return domainList[0], nil
}

func (r *DomainRepo) applyDomainListFilterAndOrder(domainList []networkingv1alpha1.CFDomain, message DomainListMessage) []networkingv1alpha1.CFDomain {
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

func (r *DomainRepo) returnDomainList(domainList []networkingv1alpha1.CFDomain) []DomainRecord {
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
		CreatedAt: cfDomain.CreationTimestamp.UTC().Format(TimestampFormat),
		UpdatedAt: updatedAtTime,
	}
}
