package presenter_test

import (
	"encoding/json"
	"net/url"

	"code.cloudfoundry.org/korifi/api/config"
	"code.cloudfoundry.org/korifi/api/presenter"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Root endpoints", func() {
	var (
		baseURL     *url.URL
		logCacheURL *url.URL
		output      []byte
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		logCacheURL, err = url.Parse("https://api.example.logcache.org")
		Expect(err).NotTo(HaveOccurred())
	})

	Context("/", func() {
		var uaaConfig config.UAA

		BeforeEach(func() {
			uaaConfig = config.UAA{}
		})

		JustBeforeEach(func() {
			response := presenter.ForRoot(*baseURL, uaaConfig, *logCacheURL)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("produces expected root json", func() {
			Expect(output).To(MatchJSON(`{
				"links": {
					"app_ssh": null,
					"bits_service": null,
					"cloud_controller_v2": null,
					"cloud_controller_v3": {
							"href": "https://api.example.org/v3",
							"meta": {
									"version": "3.117.0+cf-k8s"
							}
					},
					"credhub": null,
					"log_cache": {
							"href": "https://api.example.logcache.org",
							"meta": {
									"version": ""
							}
					},
					"log_stream": null,
					"logging": null,
					"login": {
							"href": "https://api.example.org",
							"meta": {
									"version": ""
							}
					},
					"network_policy_v0": null,
					"network_policy_v1": null,
					"routing": null,
					"self": {
							"href": "https://api.example.org",
							"meta": {
									"version": ""
							}
					},
					"uaa": null
				},
				"cf_on_k8s": true
			}`))
		})

		When("UAA support is enabled", func() {
			BeforeEach(func() {
				uaaConfig = config.UAA{
					Enabled: true,
					URL:     "https://my.uaa",
				}
			})

			It("produces expected root json", func() {
				Expect(output).To(MatchJSON(`{
				"links": {
					"app_ssh": null,
					"bits_service": null,
					"cloud_controller_v2": null,
					"cloud_controller_v3": {
							"href": "https://api.example.org/v3",
							"meta": {
									"version": "3.117.0+cf-k8s"
							}
					},
					"credhub": null,
					"log_cache": {
							"href": "https://api.example.logcache.org",
							"meta": {
									"version": ""
							}
					},
					"log_stream": null,
					"logging": null,
					"login": {
							"href": "https://my.uaa",
							"meta": {
									"version": ""
							}
					},
					"network_policy_v0": null,
					"network_policy_v1": null,
					"routing": null,
					"self": {
							"href": "https://api.example.org",
							"meta": {
									"version": ""
							}
					},
					"uaa": {
							"href": "https://my.uaa",
							"meta": {
									"version": ""
							}
					}
				},
				"cf_on_k8s": false
			}`))
			})
		})
	})

	Context("/v3", func() {
		JustBeforeEach(func() {
			response := presenter.ForRootV3(*baseURL)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("produces expected root v3 json", func() {
			Expect(output).To(MatchJSON(`{
				"links": {
					"self": {
						"href": "https://api.example.org/v3"
					}
				}
			}`))
		})
	})
})
