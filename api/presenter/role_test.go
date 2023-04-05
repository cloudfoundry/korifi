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

var _ = Describe("Role", func() {
	var (
		baseURL *url.URL
		output  []byte
		record  repositories.RoleRecord
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		record = repositories.RoleRecord{
			GUID:      "the-role-guid",
			CreatedAt: "2021-09-17T15:23:10Z",
			UpdatedAt: "2021-09-17T15:23:10Z",
			Type:      "space_developer",
			User:      "the-user",
			Kind:      "",
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForRole(record, *baseURL)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces expected role json", func() {
		Expect(output).To(MatchJSON(`{
			"guid": "the-role-guid",
			"created_at": "2021-09-17T15:23:10Z",
			"updated_at": "2021-09-17T15:23:10Z",
			"type": "space_developer",
			"relationships": {
				"user": {
					"data":{
						"guid": "the-user"
					}
				},
				"space": {
					"data": null
				},
				"organization": {
					"data": null
				}
			},
			"links": {
				"self": {
					"href": "https://api.example.org/v3/roles/the-role-guid"
				},
				"user": {
					"href": "https://api.example.org/v3/users/the-user"
				}
			}
		}`))
	})

	When("presenting an org role", func() {
		BeforeEach(func() {
			record.Org = "the-org-guid"
		})

		It("includes the org in relationships and links", func() {
			Expect(output).To(MatchJSONPath("$.relationships.organization.data.guid", "the-org-guid"))
			Expect(output).To(MatchJSONPath("$.links.organization.href", "https://api.example.org/v3/organizations/the-org-guid"))
		})
	})

	When("presenting a space role", func() {
		BeforeEach(func() {
			record.Space = "the-space-guid"
		})

		It("includes the space in relationships and links", func() {
			Expect(output).To(MatchJSONPath("$.relationships.space.data.guid", "the-space-guid"))
			Expect(output).To(MatchJSONPath("$.links.space.href", "https://api.example.org/v3/spaces/the-space-guid"))
		})
	})
})
