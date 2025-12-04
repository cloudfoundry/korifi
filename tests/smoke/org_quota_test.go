package smoke_test

import (
	"code.cloudfoundry.org/korifi/tests/helpers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("OrgQuotas", func() {
	var orgQuotaName string
	BeforeEach(func() {
		orgQuotaName = uuid.NewString()
	})

	Describe("cf org-quota", func() {
		It("returns OK", func() {
			session := helpers.Cf("org-quota", orgQuotaName)
			Expect(session).To(Exit(0))
		})
	})
})
