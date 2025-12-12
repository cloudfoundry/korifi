package smoke_test

import (
	"code.cloudfoundry.org/korifi/tests/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("OrgQuotas", func() {
	Describe("cf org-quota", func() {
		It("returns OK", func() {
			session := helpers.Cf("org-quotas")
			Expect(session).To(Exit(0))
		})
	})
})
