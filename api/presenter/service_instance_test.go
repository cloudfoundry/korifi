package presenter_test

import (
	"encoding/json"
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service Instance", func() {
	var (
		baseURL *url.URL
		output  []byte
		record  repositories.ServiceInstanceRecord
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		record = repositories.ServiceInstanceRecord{
			Name:      "service-instance-name",
			GUID:      "service-instance-guid",
			PlanGUID:  "service-plan-guid",
			SpaceGUID: "space-guid",
			Tags:      []string{"foo", "bar"},
			Type:      "user-provided",
			CreatedAt: time.UnixMilli(1000),
			UpdatedAt: tools.PtrTo(time.UnixMilli(2000)),
			Labels: map[string]string{
				"foo": "bar",
			},
			LastOperation: korifiv1alpha1.LastOperation{
				Type:        "update",
				State:       "succeeded",
				Description: "Operation succeeded",
			},
			Annotations: map[string]string{
				"one": "two",
			},
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForServiceInstance(record, *baseURL)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns the expected JSON", func() {
		Expect(output).To(MatchJSON(`{
			"guid": "service-instance-guid",
			"name": "service-instance-name",
			"type": "user-provided",
			"links": {
				"credentials": {
					"href": "https://api.example.org/v3/service_instances/service-instance-guid/credentials"
				},
				"self": {
					"href": "https://api.example.org/v3/service_instances/service-instance-guid"
				},
				"service_credential_bindings": {
					"href": "https://api.example.org/v3/service_credential_bindings?service_instance_guids=service-instance-guid"
				},
				"service_plan": {
					"href": "https://api.example.org/v3/service_plans/service-plan-guid"
				},
				"service_route_bindings": {
					"href": "https://api.example.org/v3/service_route_bindings?service_instance_guids=service-instance-guid"
				},
				"space": {
					"href": "https://api.example.org/v3/spaces/space-guid"
				}
			},
			"last_operation": {
				"created_at": "1970-01-01T00:00:01Z",
				"updated_at": "1970-01-01T00:00:02Z",
				"description": "Operation succeeded",
				"state": "succeeded",
				"type": "update"
			},
			"metadata": {
				"annotations": {
					"one": "two"
				},
				"labels": {
					"foo": "bar"
				}
			},
			"relationships": {
				"space": {
					"data": {
						"guid": "space-guid"
					}
				}
			},
			"route_service_url": null,
			"syslog_drain_url": null,
			"tags": [
				"foo",
				"bar"
			],
			"created_at": "1970-01-01T00:00:01Z",
			"updated_at": "1970-01-01T00:00:02Z"
		}`))
	})

	When("labels is nil", func() {
		BeforeEach(func() {
			record.Labels = nil
		})

		It("returns an empty slice of labels", func() {
			Expect(output).To(MatchJSONPath("$.metadata.labels", Not(BeNil())))
		})
	})

	When("annotations is nil", func() {
		BeforeEach(func() {
			record.Annotations = nil
		})

		It("returns an empty slice of annotations", func() {
			Expect(output).To(MatchJSONPath("$.metadata.annotations", Not(BeNil())))
		})
	})

	When("the service instance is managed", func() {
		BeforeEach(func() {
			record.Type = "managed"
			record.MaintenanceInfo = repositories.MaintenanceInfo{
				Version: "1.2.3",
			}
			record.UpgradeAvailable = true
		})

		It("returns maintenance_info", func() {
			Expect(output).To(MatchJSONPath("$.maintenance_info.version", Equal("1.2.3")))
		})

		It("returns upgrade_available", func() {
			Expect(output).To(MatchJSONPath("$.upgrade_available", BeTrue()))
		})
	})
})
