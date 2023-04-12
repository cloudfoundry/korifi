package presenter_test

import (
	"encoding/json"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Space", func() {
	var (
		baseURL *url.URL
		output  []byte
		record  repositories.SpaceRecord
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		record = repositories.SpaceRecord{
			Name:             "the-space",
			GUID:             "the-space-guid",
			OrganizationGUID: "the-org-guid",
			Labels: map[string]string{
				"label-key": "label-val",
			},
			Annotations: map[string]string{
				"annotation-key": "annotation-val",
			},
			CreatedAt: "then",
			UpdatedAt: "later",
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForSpace(record, *baseURL)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces expected space json", func() {
		Expect(output).To(MatchJSON(`{
			"guid": "the-space-guid",
			"name": "the-space",
			"created_at": "then",
			"updated_at": "later",
			"metadata": {
				"labels": {
					"label-key": "label-val"
				},
				"annotations": {
					"annotation-key": "annotation-val"
				}
			},
			"relationships": {
				"organization": {
					"data": {
						"guid": "the-org-guid"
					}
				}
			},
			"links": {
				"self": {
					"href": "https://api.example.org/v3/spaces/the-space-guid"
				},
				"organization": {
					"href": "https://api.example.org/v3/organizations/the-org-guid"
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
