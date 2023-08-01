package presenter_test

import (
	"encoding/json"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("User", func() {
	var (
		baseURL *url.URL
		output  []byte
		name    string
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		name = "bob"
	})

	JustBeforeEach(func() {
		response := presenter.ForUser(name, *baseURL)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces expected user json", func() {
		Expect(output).To(MatchJSON(`{
			"guid": "bob",
			"username": "bob"
		}`))
	})
})
