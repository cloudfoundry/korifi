package presenter_test

import (
	"encoding/json"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	. "code.cloudfoundry.org/korifi/tests/matchers"
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
			listResult        repositories.ListResult[record]
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

			listResult = repositories.ListResult[record]{
				PageInfo: descriptors.PageInfo{
					TotalResults: 10,
					TotalPages:   5,
					PageNumber:   3,
					PageSize:     2,
				},
				Records: []record{{N: 42}, {N: 43}},
			}
			includedResources = []include.Resource{}
		})

		JustBeforeEach(func() {
			response := presenter.ForList(forRecord, listResult, *baseURL, *requestURL, includedResources...)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected json", func() {
			Expect(output).To(MatchJSON(`{
				"pagination": {
					"total_results": 10,
					"total_pages": 5,
					"first": {
						"href": "https://api.example.org/v3/records?foo=bar&page=1&per_page=2"
					},
					"last": {
						"href": "https://api.example.org/v3/records?foo=bar&page=5&per_page=2"
					},
					"next": {
						"href": "https://api.example.org/v3/records?foo=bar&page=4&per_page=2"
					},
					"previous": {
						"href": "https://api.example.org/v3/records?foo=bar&page=2&per_page=2"
					}
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

		When("page number is greater than total pages", func() {
			BeforeEach(func() {
				listResult.PageInfo.PageNumber = 6
			})

			It("does not return next", func() {
				Expect(output).To(MatchJSONPath("$.pagination.next", BeNil()))
			})

			It("returns previous referencing the last page", func() {
				Expect(output).To(MatchJSONPath("$.pagination.previous.href", Equal("https://api.example.org/v3/records?foo=bar&page=5&per_page=2")))
			})
		})

		When("page number is 1", func() {
			BeforeEach(func() {
				listResult.PageInfo.PageNumber = 1
			})

			It("does not return previous", func() {
				Expect(output).To(MatchJSONPath("$.pagination.previous", BeNil()))
			})
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
					"total_results": 10,
					"total_pages": 5,
					"first": {
						"href": "https://api.example.org/v3/records?foo=bar&page=1&per_page=2"
					},
					"last": {
						"href": "https://api.example.org/v3/records?foo=bar&page=5&per_page=2"
					},
					"next": {
						"href": "https://api.example.org/v3/records?foo=bar&page=4&per_page=2"
					},
					"previous": {
						"href": "https://api.example.org/v3/records?foo=bar&page=2&per_page=2"
					}
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

		When("there are no results", func() {
			BeforeEach(func() {
				listResult = repositories.ListResult[record]{
					PageInfo: descriptors.PageInfo{
						PageSize: 2,
					},
				}
			})

			It("returns an empty response", func() {
				Expect(output).To(MatchJSON(`{
					"pagination": {
						"total_results": 0,
						"total_pages": 0,
						"first": {
							"href": "https://api.example.org/v3/records?foo=bar&page=1&per_page=2"
						},
						"last": {
							"href": "https://api.example.org/v3/records?foo=bar&page=1&per_page=2"
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
