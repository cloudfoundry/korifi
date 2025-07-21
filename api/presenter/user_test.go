package presenter_test

import (
	"encoding/json"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("User", func() {
	var (
		userRecord repositories.UserRecord
		output     []byte
	)

	BeforeEach(func() {
		userRecord = repositories.UserRecord{
			GUID: "bob-guid",
			Name: "bob",
		}
	})

	JustBeforeEach(func() {
		baseURL, err := url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())

		response := presenter.ForUser(userRecord, *baseURL)
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns user response", func() {
		Expect(output).To(MatchJSON(`{
			"guid": "bob-guid",
			"username": "bob",
			"links": {
			  "self": {
				"href": "https://api.example.org/v3/users/bob-guid"
			  }
			}
		}`))
	})
})
