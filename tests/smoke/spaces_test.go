package smoke_test

import (
	"code.cloudfoundry.org/korifi/tests/helpers"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("Spaces", func() {
	var spaceName string

	BeforeEach(func() {
		spaceName = uuid.NewString()
		Expect(helpers.Cf(
			"create-space",
			spaceName,
		)).To(Exit(0))
	})

	Describe("cf rename-space", func() {
		It("renames the space", func() {
			session := helpers.Cf("renamed-space", spaceName, "renamed-space")
			Expect(session).To(Exit(0))

			session = helpers.Cf("spaces")
			Expect(session).To(Exit(0))

			lines := it.MustCollect(it.LinesString(session.Out))
			Expect(lines).To(ContainElement(
				matchSubstrings("renamed-space"),
			))
		})
	})
})
