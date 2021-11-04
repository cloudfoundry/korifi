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
	Name      string
	GUID      string
	CreatedAt string
	UpdatedAt string
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

	return f.cfDomainToDomainRecord(domain), nil
}

func (f *DomainRepo) cfDomainToDomainRecord(cfDomain *networkingv1alpha1.CFDomain) DomainRecord {
	return DomainRecord{
		Name:      cfDomain.Spec.Name,
		GUID:      cfDomain.Name,
		CreatedAt: "",
		UpdatedAt: "",
	}
}
