package repositories

import (
	"context"
	"fmt"
	"slices"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ServiceOfferingResourceType = "Service Offering"

type ServiceOfferingRecord struct {
	services.ServiceOffering
	model.CFResource
	ServiceBrokerGUID string
}

func (r ServiceOfferingRecord) Relationships() map[string]string {
	return map[string]string{
		"service_broker": r.ServiceBrokerGUID,
	}
}

type ServiceOfferingRepo struct {
	userClientFactory authorization.UserK8sClientFactory
	rootNamespace     string
	brokerRepo        *ServiceBrokerRepo
}

type ListServiceOfferingMessage struct {
	Names       []string
	GUIDs       []string
	BrokerNames []string
}

func (m *ListServiceOfferingMessage) matches(cfServiceOffering korifiv1alpha1.CFServiceOffering) bool {
	return tools.EmptyOrContains(m.Names, cfServiceOffering.Spec.Name) &&
		tools.EmptyOrContains(m.GUIDs, cfServiceOffering.Name) &&
		tools.EmptyOrContains(m.BrokerNames, cfServiceOffering.Labels[korifiv1alpha1.RelServiceBrokerNameLabel])
}

func NewServiceOfferingRepo(
	userClientFactory authorization.UserK8sClientFactory,
	rootNamespace string,
	brokerRepo *ServiceBrokerRepo,
) *ServiceOfferingRepo {
	return &ServiceOfferingRepo{
		userClientFactory: userClientFactory,
		rootNamespace:     rootNamespace,
		brokerRepo:        brokerRepo,
	}
}

func (r *ServiceOfferingRepo) ListOfferings(ctx context.Context, authInfo authorization.Info, message ListServiceOfferingMessage) ([]ServiceOfferingRecord, error) {
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

	return slices.Collect(it.Map(itx.FromSlice(offeringsList.Items).Filter(message.matches), offeringToRecord)), nil
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
		ServiceBrokerGUID: offering.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel],
	}
}
