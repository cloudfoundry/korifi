package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

var _ = Describe("Stack", func() {
	Describe("StackList", func() {
		Describe("Validation", func() {
			DescribeTable("valid query",
				func(query string, expectedStackList payloads.StackList) {
					actualStackList, decodeErr := decodeQuery[payloads.StackList](query)

					Expect(decodeErr).NotTo(HaveOccurred())
					Expect(*actualStackList).To(Equal(expectedStackList))
				},

				Entry("page=3", "page=3", payloads.StackList{Pagination: payloads.Pagination{Page: "3"}}),
			)
			DescribeTable("invalid query",
				func(query string, errMatcher types.GomegaMatcher) {
					_, decodeErr := decodeQuery[payloads.StackList](query)
					Expect(decodeErr).To(errMatcher)
				},

				Entry("per_page is not a number", "per_page=foo", MatchError(ContainSubstring("value must be an integer"))),
			)
		})

		Describe("ToMessage", func() {
			It("splits names to strings", func() {
				stackList := payloads.StackList{
					Pagination: payloads.Pagination{
						PerPage: "20",
						Page:    "1",
					},
				}
				Expect(stackList.ToMessage()).To(Equal(repositories.ListStacksMessage{
					Pagination: repositories.Pagination{
						PerPage: 20,
						Page:    1,
					},
				}))
			})
		})
	})
})
