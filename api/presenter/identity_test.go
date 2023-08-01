package presenter_test

import (
	"encoding/json"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/presenter"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Identity", func() {
	var (
		output []byte
		id     authorization.Identity
	)

	BeforeEach(func() {
		id = authorization.Identity{
			Name: "the-user",
			Kind: "User",
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForWhoAmI(id)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces expected identity json", func() {
		Expect(output).To(MatchJSON(`{
			"name": "the-user",
			"kind": "User"
		}`))
	})
})
