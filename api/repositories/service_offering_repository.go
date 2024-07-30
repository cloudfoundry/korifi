package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	"github.com/BooleanCat/go-functional/iter"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ServiceOfferingResourceType = "Service Offering"

type ServiceOfferingRecord struct {
	services.ServiceOffering
	model.CFResource
	Relationships ServiceOfferingRelationships `json:"relationships"`
}

type ServiceOfferingRelationships struct {
	ServiceBroker model.ToOneRelationship `json:"service_broker"`
}

type ServiceOfferingRepo struct {
	userClientFactory authorization.UserK8sClientFactory
	rootNamespace     string
}

func NewServiceOfferingRepo(
	userClientFactory authorization.UserK8sClientFactory,
	rootNamespace string,
) *ServiceOfferingRepo {
	return &ServiceOfferingRepo{
		userClientFactory: userClientFactory,
		rootNamespace:     rootNamespace,
	}
}

func (r *ServiceOfferingRepo) ListOfferings(ctx context.Context, authInfo authorization.Info) ([]ServiceOfferingRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []ServiceOfferingRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	offeringsList := &korifiv1alpha1.CFServiceOfferingList{}
	err = userClient.List(ctx, offeringsList, client.InNamespace(r.rootNamespace))
	if err != nil {
		if k8serrors.IsForbidden(err) {
			return []ServiceOfferingRecord{}, nil
		}

		return []ServiceOfferingRecord{}, fmt.Errorf("failed to list service offerings: %w",
			apierrors.FromK8sError(err, ServiceOfferingResourceType),
		)
	}

	return iter.Map(iter.Lift(offeringsList.Items), offeringToRecord).Collect(), nil
}

func offeringToRecord(offering korifiv1alpha1.CFServiceOffering) ServiceOfferingRecord {
	return ServiceOfferingRecord{
		ServiceOffering: offering.Spec.ServiceOffering,
		CFResource: model.CFResource{
			GUID:      offering.Name,
			CreatedAt: offering.CreationTimestamp.Time,
			Metadata: model.Metadata{
				Labels:      offering.Labels,
				Annotations: offering.Annotations,
			},
		},
		Relationships: ServiceOfferingRelationships{
			ServiceBroker: model.ToOneRelationship{
				Data: model.Relationship{
					GUID: offering.Labels[korifiv1alpha1.RelServiceBrokerLabel],
				},
			},
		},
	}
}
