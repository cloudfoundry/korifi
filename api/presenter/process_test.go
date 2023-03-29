package presenter_test

import (
	"encoding/json"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Process", func() {
	var (
		baseURL *url.URL
		output  []byte
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Process Response", func() {
		var record repositories.ProcessRecord

		BeforeEach(func() {
			record = repositories.ProcessRecord{
				GUID:             "process-guid",
				SpaceGUID:        "space-guid",
				AppGUID:          "app-guid",
				Type:             "web",
				Command:          "rackup",
				DesiredInstances: 5,
				MemoryMB:         256,
				DiskQuotaMB:      1024,
				Ports:            []int32{8080},
				HealthCheck: repositories.HealthCheck{
					Type: "port",
				},
				Labels: map[string]string{
					"label-key": "label-val",
				},
				Annotations: map[string]string{
					"annotation-key": "annotation-val",
				},
				CreatedAt: "2016-03-23T18:48:22Z",
				UpdatedAt: "2016-03-23T18:48:42Z",
			}
		})

		JustBeforeEach(func() {
			response := presenter.ForProcess(record, *baseURL)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected JSON", func() {
			Expect(output).Should(MatchJSON(`{
				"guid": "process-guid",
				"type": "web",
				"command": "rackup",
				"instances": 5,
				"memory_in_mb": 256,
				"disk_in_mb": 1024,
				"health_check": {
					"type": "port",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
				},
				"relationships": {
					"app": {
						"data": {
							"guid": "app-guid"
						}
					}
				},
				"metadata": {
					"labels": {
						"label-key": "label-val"
					},
					"annotations": {
						"annotation-key": "annotation-val"
					}
				},
				"created_at": "2016-03-23T18:48:22Z",
				"updated_at": "2016-03-23T18:48:42Z",
				"links": {
					"self": {
						"href": "https://api.example.org/v3/processes/process-guid"
					},
					"scale": {
						"href": "https://api.example.org/v3/processes/process-guid/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://api.example.org/v3/apps/app-guid"
					},
					"space": {
						"href": "https://api.example.org/v3/spaces/space-guid"
					},
					"stats": {
						"href": "https://api.example.org/v3/processes/process-guid/stats"
					}
				}
			}`))
		})
	})
})
