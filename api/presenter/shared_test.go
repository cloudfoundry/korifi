package presenter_test

import (
	"encoding/json"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories/include"
)

type (
	record struct {
		N int
	}

	presentedRecord struct {
		M int    `json:"m"`
		U string `json:"u"`
	}
)

func forRecord(r record, u url.URL, includes ...include.Resource) presentedRecord {
	return presentedRecord{
		M: r.N,
		U: u.String(),
	}
}

var _ = Describe("Shared", func() {
	Describe("ForList", func() {
		var (
			records           []record
			includedResources []include.Resource
			baseURL           *url.URL
			requestURL        *url.URL
			output            []byte
		)

		BeforeEach(func() {
			var err error
			baseURL, err = url.Parse("https://api.example.org")
			Expect(err).NotTo(HaveOccurred())

			requestURL, err = url.Parse("https://api.example.org/v3/records?foo=bar")
			Expect(err).NotTo(HaveOccurred())

			records = []record{{N: 42}, {N: 43}}
			includedResources = []include.Resource{}
		})

		JustBeforeEach(func() {
			response := presenter.ForList(forRecord, records, *baseURL, *requestURL, includedResources...)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected json", func() {
			Expect(output).To(MatchJSON(`{
				"pagination": {
					"total_results": 2,
					"total_pages": 1,
					"first": {
						"href": "https://api.example.org/v3/records?foo=bar"
					},
					"last": {
						"href": "https://api.example.org/v3/records?foo=bar"
					},
					"next": null,
					"previous": null
				},
				"resources": [
					{
						"m": 42,
						"u": "https://api.example.org"
					},
					{
						"m": 43,
						"u": "https://api.example.org"
					}
				]
			}`))
		})

		When("included resources are provided", func() {
			BeforeEach(func() {
				includedResources = []include.Resource{
					{
						Type:     "typeA",
						Resource: map[string]string{"a": "A"},
					},
					{
						Type:     "typeA",
						Resource: map[string]string{"a1": "A1"},
					},
					{
						Type:     "typeB",
						Resource: map[string]string{"b": "B"},
					},
				}
			})

			It("returns the expected json", func() {
				Expect(output).To(MatchJSON(`{
				  "pagination": {
					"total_results": 2,
					"total_pages": 1,
					"first": {
					  "href": "https://api.example.org/v3/records?foo=bar"
					},
					"last": {
					  "href": "https://api.example.org/v3/records?foo=bar"
					},
					"next": null,
					"previous": null
				  },
				  "resources": [
					{
					  "m": 42,
					  "u": "https://api.example.org"
					},
					{
					  "m": 43,
					  "u": "https://api.example.org"
					}
				  ],
				  "included": {
					"typeA": [
					  {
						"a": "A"
					  },
					  {
						"a1": "A1"
					  }
					],
					"typeB": [
					  {
						"b": "B"
					  }
					]
				  }
				}
				`))
			})
		})

		When("records are empty", func() {
			BeforeEach(func() {
				records = nil
			})

			It("returns an empty response", func() {
				Expect(output).To(MatchJSON(`{
					"pagination": {
						"total_results": 0,
						"total_pages": 1,
						"first": {
							"href": "https://api.example.org/v3/records?foo=bar"
						},
						"last": {
							"href": "https://api.example.org/v3/records?foo=bar"
						},
						"next": null,
						"previous": null
					},
					"resources": []
				}`))
			})
		})
	})

	Describe("ForRelationships", func() {
		It("presents relationships", func() {
			Expect(presenter.ForRelationships(map[string]string{
				"foo": "bar",
			})).To(Equal(map[string]presenter.ToOneRelationship{
				"foo": {
					Data: presenter.Relationship{
						GUID: "bar",
					},
				},
			}))
		})
	})
})
