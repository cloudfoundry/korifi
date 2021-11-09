package repositories

import (
	"context"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfdomains,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfdomains/status,verbs=get

type DomainRepo struct{}

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

func (f *DomainRepo) FetchDomain(ctx context.Context, client client.Client, domainGUID string) (DomainRecord, error) {
	domain := &networkingv1alpha1.CFDomain{}
	err := client.Get(ctx, types.NamespacedName{Name: domainGUID}, domain)
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

func (f *DomainRepo) FetchDomainList(ctx context.Context, client client.Client, message DomainListMessage) ([]DomainRecord, error) {
	cfdomainList := &networkingv1alpha1.CFDomainList{}
	err := client.List(ctx, cfdomainList)

	if err != nil {
		return []DomainRecord{}, err
	}

	filtered := f.applyFilter(cfdomainList.Items, message)

	return f.returnDomainList(filtered), nil
}

func (f *DomainRepo) applyFilter(domainList []networkingv1alpha1.CFDomain, message DomainListMessage) []networkingv1alpha1.CFDomain {
	if len(message.Names) == 0 {
		return domainList
	}

	var filtered []networkingv1alpha1.CFDomain
	for _, domain := range domainList {
		for _, name := range message.Names {
			if domain.Spec.Name == name {
				filtered = append(filtered, domain)
			}
		}
	}

	return filtered
}

func (f *DomainRepo) returnDomainList(domainList []networkingv1alpha1.CFDomain) []DomainRecord {
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
