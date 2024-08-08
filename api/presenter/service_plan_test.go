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
				ServicePlan: services.ServicePlan{
					Name:        "my-service-plan",
					Free:        true,
					Description: "service plan description",
					BrokerCatalog: services.ServicePlanBrokerCatalog{
						ID: "broker-catalog-plan-guid",
						Metadata: &runtime.RawExtension{
							Raw: []byte(`{"foo":"bar"}`),
						},
						Features: services.ServicePlanFeatures{
							PlanUpdateable: true,
							Bindable:       true,
						},
					},
					Schemas: services.ServicePlanSchemas{
						ServiceInstance: services.ServiceInstanceSchema{
							Create: services.InputParameterSchema{
								Parameters: &runtime.RawExtension{
									Raw: []byte(`{"create-param":"create-value"}`),
								},
							},
							Update: services.InputParameterSchema{
								Parameters: &runtime.RawExtension{
									Raw: []byte(`{"update-param":"update-value"}`),
								},
							},
						},
						ServiceBinding: services.ServiceBindingSchema{
							Create: services.InputParameterSchema{
								Parameters: &runtime.RawExtension{
									Raw: []byte(`{"binding-create-param":"binding-create-value"}`),
								},
							},
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
					Organizations: []services.VisibilityOrganization{{
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
