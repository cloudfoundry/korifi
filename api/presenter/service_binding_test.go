package presenter_test

import (
	"encoding/json"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service Binding", func() {
	var (
		baseURL *url.URL
		output  []byte
		record  repositories.ServiceBindingRecord
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		record = repositories.ServiceBindingRecord{
			GUID:                "binding-guid",
			Type:                "user-provided",
			Name:                tools.PtrTo("binding-name"),
			AppGUID:             "app-guid",
			ServiceInstanceGUID: "service-instance-guid",
			SpaceGUID:           "space-guid",
			Labels: map[string]string{
				"label-key": "label-val",
			},
			Annotations: map[string]string{
				"annotation-key": "annotation-key",
			},
			CreatedAt: "then",
			UpdatedAt: "now",
			LastOperation: repositories.ServiceBindingLastOperation{
				Type:        "hernia",
				State:       "ruptured",
				Description: tools.PtrTo("bad"),
				CreatedAt:   "yesterday",
				UpdatedAt:   "later",
			},
		}
	})

	Describe("ForServiceBinding", func() {
		JustBeforeEach(func() {
			response := presenter.ForServiceBinding(record, *baseURL)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected JSON", func() {
			Expect(output).To(MatchJSON(`{
				"guid": "binding-guid",
				"type": "user-provided",
				"name": "binding-name",
				"created_at": "then",
				"updated_at": "now",
				"last_operation": {
					"type": "hernia",
					"state": "ruptured",
					"description": "bad",
					"created_at": "yesterday",
					"updated_at": "later"
				},
				"relationships": {
					"app": {
						"data": {
							"guid": "app-guid"
						}
					},
					"service_instance": {
						"data": {
							"guid": "service-instance-guid"
						}
					}
				},
				"links": {
					"app": {
						"href": "https://api.example.org/v3/apps/app-guid"
					},
					"service_instance": {
						"href": "https://api.example.org/v3/service_instances/service-instance-guid"
					},
					"self": {
						"href": "https://api.example.org/v3/service_credential_bindings/binding-guid"
					},
					"details": {
						"href": "https://api.example.org/v3/service_credential_bindings/binding-guid/details"
					}
				},
				"metadata": {
					"labels": {
						"label-key": "label-val"
					},
					"annotations": {
						"annotation-key": "annotation-key"
					}
				}
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
	})

	Describe("ForServiceBindingList", func() {
		var (
			otherRecord repositories.ServiceBindingRecord
			app         repositories.AppRecord
			requestURL  *url.URL
		)

		BeforeEach(func() {
			otherRecord = record
			otherRecord.GUID = "other-binding-guid"

			app = repositories.AppRecord{
				GUID: "app-guid",
			}

			var err error
			requestURL, err = url.Parse("https://api.example.org/v3/service_credential_bindings?foo=bar")
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			response := presenter.ForServiceBindingList([]repositories.ServiceBindingRecord{record, otherRecord}, []repositories.AppRecord{app}, *baseURL, *requestURL)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected JSON", func() {
			Expect(output).To(MatchJSONPath("$.pagination.total_results", BeEquivalentTo(2)))
			Expect(output).To(MatchJSONPath("$.resources[0].guid", "binding-guid"))
			Expect(output).To(MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/service_credential_bindings/binding-guid"))
			Expect(output).To(MatchJSONPath("$.resources[1].guid", "other-binding-guid"))
			Expect(output).To(MatchJSONPath("$.resources[1].links.self.href", "https://api.example.org/v3/service_credential_bindings/other-binding-guid"))
			Expect(output).To(MatchJSONPath("$.included.apps[0].guid", "app-guid"))
			Expect(output).To(MatchJSONPath("$.included.apps[0].links.self.href", "https://api.example.org/v3/apps/app-guid"))
		})
	})
})
