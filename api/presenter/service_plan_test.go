package presenter_test

import (
	"encoding/json"
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service Plan", func() {
	var (
		baseURL *url.URL
		output  []byte
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("ForServicePlan", func() {
		var record repositories.ServicePlanRecord

		BeforeEach(func() {
			record = repositories.ServicePlanRecord{
				Name:        "my-service-plan",
				Free:        true,
				Description: "service plan description",
				BrokerCatalog: repositories.ServicePlanBrokerCatalog{
					ID: "broker-catalog-plan-guid",
					Metadata: map[string]any{
						"foo": "bar",
					},
					Features: repositories.ServicePlanFeatures{
						PlanUpdateable: true,
						Bindable:       true,
					},
				},
				Schemas: repositories.ServicePlanSchemas{
					ServiceInstance: repositories.ServiceInstanceSchema{
						Create: repositories.InputParameterSchema{
							Parameters: map[string]any{
								"create-param": "create-value",
							},
						},
						Update: repositories.InputParameterSchema{
							Parameters: map[string]any{
								"update-param": "update-value",
							},
						},
					},
					ServiceBinding: repositories.ServiceBindingSchema{
						Create: repositories.InputParameterSchema{
							Parameters: map[string]any{
								"binding-create-param": "binding-create-value",
							},
						},
					},
				},
				MaintenanceInfo: repositories.MaintenanceInfo{
					Version: "1.2.3",
				},
				GUID:      "resource-guid",
				CreatedAt: time.UnixMilli(1000).UTC(),
				UpdatedAt: tools.PtrTo(time.UnixMilli(2000).UTC()),
				Metadata: repositories.Metadata{
					Labels: map[string]string{
						"label": "label-foo",
					},
					Annotations: map[string]string{
						"annotation": "annotation-bar",
					},
				},
				Visibility: repositories.PlanVisibility{
					Type: "visibility-type",
				},
				ServiceOfferingGUID: "service-offering-guid",
				Available:           true,
			}
		})

		JustBeforeEach(func() {
			response := presenter.ForServicePlan(record, *baseURL)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected JSON", func() {
			Expect(output).To(MatchJSON(`{
				"name": "my-service-plan",
				"free": true,
				"description": "service plan description",
				"broker_catalog": {
				  "id": "broker-catalog-plan-guid",
				  "metadata": {
					"foo": "bar"
				  },
				  "features": {
					"plan_updateable": true,
					"bindable": true
				  }
				},
				"schemas": {
				  "service_instance": {
					"create": {
					  "parameters": {
						"create-param": "create-value"
					  }
					},
					"update": {
					  "parameters": {
						"update-param": "update-value"
					  }
					}
				  },
				  "service_binding": {
					"create": {
					  "parameters": {
						"binding-create-param": "binding-create-value"
					  }
					}
				  }
				},
				"maintenance_info": {
					"version": "1.2.3"
				},
				"guid": "resource-guid",
				"visibility_type": "visibility-type",
				"available": true,
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
				  "service_offering": {
					"data": {
					  "guid": "service-offering-guid"
					}
				  }
				},
				"links": {
				  "self": {
					"href": "https://api.example.org/v3/service_plans/resource-guid"
				  },
				  "service_offering": {
					"href": "https://api.example.org/v3/service_offerings/service-offering-guid"
				  },
				  "visibility": {
					"href": "https://api.example.org/v3/service_plans/resource-guid/visibility"
				  }
				}
			}`))
		})
	})

	Describe("ForServicePlanVisibility", func() {
		var record repositories.ServicePlanRecord

		BeforeEach(func() {
			record = repositories.ServicePlanRecord{
				Visibility: repositories.PlanVisibility{
					Type: "organization",
					Organizations: []repositories.VisibilityOrganization{{
						GUID: "org-guid",
						Name: "org-name",
					}},
				},
			}
		})

		JustBeforeEach(func() {
			response := presenter.ForServicePlanVisibility(record, url.URL{})
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected JSON", func() {
			Expect(output).To(MatchJSON(`{
				"type": "organization",
				"organizations": [
				  {
					  "guid": "org-guid",
					  "name": "org-name"
				  }
				]
			}`))
		})
	})
})
