package presenter_test

import (
	"encoding/json"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Route", func() {
	var (
		baseURL *url.URL
		output  []byte
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Route Response", func() {
		var record repositories.RouteRecord

		BeforeEach(func() {
			record = repositories.RouteRecord{
				GUID:      "test-route-guid",
				SpaceGUID: "test-space-guid",
				Domain: repositories.DomainRecord{
					GUID: "test-domain-guid",
					Name: "example.org",
				},
				Host:         "test-route-host",
				Path:         "/some_path",
				Protocol:     "http",
				Destinations: nil,
				Labels:       nil,
				Annotations:  nil,
				CreatedAt:    "2019-05-10T17:17:48Z",
				UpdatedAt:    "2019-05-10T17:17:48Z",
			}
		})

		JustBeforeEach(func() {
			response := presenter.ForRoute(record, *baseURL)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected JSON", func() {
			Expect(output).Should(MatchJSON(`{
				"guid": "test-route-guid",
				"port": null,
				"path": "/some_path",
				"protocol": "http",
				"host": "test-route-host",
				"url": "test-route-host.example.org/some_path",
				"created_at": "2019-05-10T17:17:48Z",
				"updated_at": "2019-05-10T17:17:48Z",
				"destinations": [],
				"relationships": {
					"space": {
						"data": {
							"guid": "test-space-guid"
						}
					},
					"domain": {
						"data": {
							"guid": "test-domain-guid"
						}
					}
				},
				"metadata": {
					"labels": {},
					"annotations": {}
				},
				"links": {
					"self":{
						"href": "https://api.example.org/v3/routes/test-route-guid"
					},
					"space":{
						"href": "https://api.example.org/v3/spaces/test-space-guid"
					},
					"domain":{
						"href": "https://api.example.org/v3/domains/test-domain-guid"
					},
					"destinations":{
						"href": "https://api.example.org/v3/routes/test-route-guid/destinations"
					}
				}
			}`))
		})
	})
})
