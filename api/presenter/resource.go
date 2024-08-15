package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/relationships"
)

type Resource struct {
	serverURL url.URL
}

func NewResource(serverURL url.URL) *Resource {
	return &Resource{
		serverURL: serverURL,
	}
}

func (r *Resource) PresentResource(resource relationships.Resource) any {
	switch res := resource.(type) {
	case repositories.ServiceBrokerRecord:
		return ForServiceBroker(res, r.serverURL)
	case repositories.ServiceOfferingRecord:
		return ForServiceOffering(res, r.serverURL)
	case repositories.ServicePlanRecord:
		return ForServicePlan(res, r.serverURL)
	default:
		return resource
	}
}
