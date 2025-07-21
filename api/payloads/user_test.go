package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

var _ = Describe("User", func() {
	Describe("UserList", func() {
		Describe("Validation", func() {
			DescribeTable("valid query",
				func(query string, expectedUserList payloads.UserList) {
					actualUserList, decodeErr := decodeQuery[payloads.UserList](query)

					Expect(decodeErr).NotTo(HaveOccurred())
					Expect(*actualUserList).To(Equal(expectedUserList))
				},

				Entry("usernames", "usernames=alice", payloads.UserList{Names: "alice"}),
				Entry("page=3", "page=3", payloads.UserList{Pagination: payloads.Pagination{Page: "3"}}),
			)

			DescribeTable("invalid query",
				func(query string, errMatcher types.GomegaMatcher) {
					_, decodeErr := decodeQuery[payloads.UserList](query)
					Expect(decodeErr).To(errMatcher)
				},

				Entry("per_page is not a number", "per_page=foo", MatchError(ContainSubstring("value must be an integer"))),
			)
		})

		Describe("ToMessage", func() {
			var (
				payload payloads.UserList
				message repositories.ListUsersMessage
			)

			BeforeEach(func() {
				payload = payloads.UserList{
					Names: "alice,bob",
					Pagination: payloads.Pagination{
						PerPage: "20",
						Page:    "1",
					},
				}
			})

			JustBeforeEach(func() {
				message = payload.ToMessage()
			})

			It("converts to repository message", func() {
				Expect(message).To(Equal(repositories.ListUsersMessage{
					Names:      []string{"alice", "bob"},
					Pagination: repositories.Pagination{PerPage: 20, Page: 1},
				}))
			})
		})
	})
})
