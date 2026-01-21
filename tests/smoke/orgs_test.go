package smoke_test

import (
	"code.cloudfoundry.org/korifi/tests/helpers"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("Orgs", func() {
	var orgName string

	BeforeEach(func() {
		orgName = uuid.NewString()
		Expect(helpers.Cf(
			"create-org",
			orgName,
		)).To(Exit(0))
	})

	Describe("cf org", func() {
		It("returns successful", func() {
			session := helpers.Cf("org", orgName)
			Expect(session).To(Exit(0))

			lines := it.MustCollect(it.LinesString(session.Out))
			Expect(lines).To(ContainElement(
				matchSubstrings(orgName),
			))
		})
	})
})
