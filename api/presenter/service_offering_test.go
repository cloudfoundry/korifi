package presenter_test

import (
	"encoding/json"
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("Service Offering", func() {
	var (
		baseURL *url.URL
		output  []byte
		record  repositories.ServiceOfferingRecord
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		record = repositories.ServiceOfferingRecord{
			ServiceOffering: services.ServiceOffering{
				Name:             "offering-name",
				Description:      "offering description",
				Tags:             []string{"t1"},
				Requires:         []string{"r1"},
				DocumentationURL: tools.PtrTo("https://doc.url"),
				BrokerCatalog: services.ServiceBrokerCatalog{
					ID: "catalog-id",
					Metadata: &runtime.RawExtension{
						Raw: []byte(`{"foo": "bar"}`),
					},
					Features: services.BrokerCatalogFeatures{
						PlanUpdateable:       true,
						Bindable:             true,
						InstancesRetrievable: true,
						BindingsRetrievable:  true,
						AllowContextUpdates:  true,
					},
				},
			},
			CFResource: model.CFResource{
				GUID:      "resource-guid",
				CreatedAt: time.UnixMilli(1000),
				UpdatedAt: tools.PtrTo(time.UnixMilli(2000)),
				Metadata: model.Metadata{
					Labels: map[string]string{
						"label": "label-foo",
					},
					Annotations: map[string]string{
						"annotation": "annotation-bar",
					},
				},
			},
			ServiceBrokerGUID: "broker-guid",
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForServiceOffering(record, *baseURL)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns the expected JSON", func() {
		Expect(output).To(MatchJSON(`{
			"name": "offering-name",
			"description": "offering description",
			"tags": [
			  "t1"
			],
			"required": [
			  "r1"
			],
			"documentation_url": "https://doc.url",
			"broker_catalog": {
			  "id": "catalog-id",
			  "metadata": {
				"foo": "bar"
			  },
			  "features": {
				"plan_updateable": true,
				"bindable": true,
				"instances_retrievable": true,
				"bindings_retrievable": true,
				"allow_context_updates": true
			  }
			},
			"guid": "resource-guid",
			"created_at": "1970-01-01T00:00:01Z",
			"updated_at": "1970-01-01T00:00:02Z",
			"metadata": {
				"labels": {
					"label": "label-foo"
				},
				"annotations": {
					"annotation": "annotation-bar"
				}
			},
			"relationships": {
			  "service_broker": {
				"data": {
				  "guid": "broker-guid"
				}
			  }
			},
			"links": {
			  "self": {
				"href": "https://api.example.org/v3/service_offerings/resource-guid"
			  },
			  "service_plans": {
				"href": "https://api.example.org/v3/service_plans?service_offering_guids=resource-guid"
			  },
			  "service_broker": {
				"href": "https://api.example.org/v3/service_brokers/broker-guid"
			  }
			}
		}`))
	})
})
