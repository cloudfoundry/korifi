package filter_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/api/repositories/filter"
)

var _ = Describe("RoutesByAppGUID", func() {
	var (
		appGUID1     string
		appGUIDs     []string
		filterResult bool
	)

	BeforeEach(func() {
		appGUID1 = uuid.NewString()
		appGUIDs = []string{appGUID1}
	})

	JustBeforeEach(func() {
		appGUID2 := uuid.NewString()
		filterResult = filter.RoutesByAppGUIDsPredicate(appGUIDs)(appGUID1 + "\n" + appGUID2)
	})

	It("returns true", func() {
		Expect(filterResult).To(BeTrue())
	})

	When("guid is not in the list", func() {
		BeforeEach(func() {
			appGUIDs = []string{"some-other-guid"}
		})

		It("returns false", func() {
			Expect(filterResult).To(BeFalse())
		})
	})
})
