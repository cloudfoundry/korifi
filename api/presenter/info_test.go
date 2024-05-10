package presenter_test

import (
	"encoding/json"
	"net/url"

	"code.cloudfoundry.org/korifi/api/config"
	"code.cloudfoundry.org/korifi/api/presenter"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Info endpoints", func() {
	var (
		baseURL    *url.URL
		infoConfig config.InfoConfig
		output     []byte
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())

		infoConfig = config.InfoConfig{
			SupportAddress: "https://www.cloudfoundry.org/technology/korifi/",
		}
	})

	Context("/v3/info", func() {
		JustBeforeEach(func() {
			response := presenter.ForInfoV3(*baseURL, infoConfig)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("produces expected info v3 json", func() {
			Expect(output).To(MatchJSON(`{
				"build": "v9999.99.99-local.dev",
				"cli_version": {
				  "minimum": "",
					"recommended": ""
				},
				"description": "",
				"name": "",
				"version": "v9999.99.99-local.dev",
				"custom": {},
				"links": {
					"self": {
							"href": "https://api.example.org/v3/info"
					},
					"support": {
							"href": "https://www.cloudfoundry.org/technology/korifi/"
					}
				}
			}`))
		})
	})
})
