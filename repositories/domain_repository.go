package repositories

import (
	"context"
	"errors"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfdomains,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfdomains/status,verbs=get

type DomainRepo struct{}

type DomainRecord struct {
	Name      string
	GUID      string
	CreatedAt string
	UpdatedAt string
}

func (f *DomainRepo) FetchDomain(ctx context.Context, client client.Client, domainGUID string) (DomainRecord, error) {
	cfDomainList := &networkingv1alpha1.CFDomainList{}
	err := client.List(ctx, cfDomainList)

	if err != nil {
		return DomainRecord{}, err
	}

	domainList := cfDomainList.Items
	filteredDomainList := f.filterByDomainName(domainList, domainGUID)

	return f.returnDomain(filteredDomainList)
}

func (f *DomainRepo) filterByDomainName(domainList []networkingv1alpha1.CFDomain, name string) []networkingv1alpha1.CFDomain {
	filtered := []networkingv1alpha1.CFDomain{}

	for i, domain := range domainList {
		if domain.Name == name {
			filtered = append(filtered, domainList[i])
		}
	}

	return filtered
}

func (f *DomainRepo) returnDomain(domainList []networkingv1alpha1.CFDomain) (DomainRecord, error) {
	if len(domainList) == 0 {
		return DomainRecord{}, NotFoundError{Err: errors.New("not found")}
	}

	return cfDomainToDomainRecord(domainList[0]), nil
}

func cfDomainToDomainRecord(cfDomain networkingv1alpha1.CFDomain) DomainRecord {
	return DomainRecord{
		Name:      cfDomain.Spec.Name,
		GUID:      cfDomain.Name,
		CreatedAt: "",
		UpdatedAt: "",
	}
}
