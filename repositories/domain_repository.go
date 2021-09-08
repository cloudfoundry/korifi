package repositories

import (
	"context"
	"errors"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

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

// TODO: Make a general ConfigureClient function / config and client generating package
func (r *DomainRepo) ConfigureClient(config *rest.Config) (client.Client, error) {
	domainClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})

	if err != nil {
		return nil, err
	}

	return domainClient, nil
}

func (f *DomainRepo) FetchDomain(client client.Client, domainGUID string) (DomainRecord, error) {
	cfDomainList := &networkingv1alpha1.CFDomainList{}
	err := client.List(context.Background(), cfDomainList)

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
