package payloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
)

var _ = Describe("BuildpackList", func() {
	Describe("Validation", func() {
		DescribeTable("valid query",
			func(query string, expectedBuildpackList payloads.BuildpackList) {
				actualBuildpackList, decodeErr := decodeQuery[payloads.BuildpackList](query)

				Expect(decodeErr).NotTo(HaveOccurred())
				Expect(*actualBuildpackList).To(Equal(expectedBuildpackList))
			},

			Entry("created_at", "order_by=created_at", payloads.BuildpackList{OrderBy: "created_at"}),
			Entry("-created_at", "order_by=-created_at", payloads.BuildpackList{OrderBy: "-created_at"}),
			Entry("updated_at", "order_by=updated_at", payloads.BuildpackList{OrderBy: "updated_at"}),
			Entry("-updated_at", "order_by=-updated_at", payloads.BuildpackList{OrderBy: "-updated_at"}),
			Entry("position", "order_by=position", payloads.BuildpackList{OrderBy: "position"}),
			Entry("-position", "order_by=-position", payloads.BuildpackList{OrderBy: "-position"}),
			Entry("empty", "order_by=", payloads.BuildpackList{OrderBy: ""}),
			Entry("page=3", "page=3", payloads.BuildpackList{Pagination: payloads.Pagination{Page: "3"}}),
		)

		DescribeTable("invalid query",
			func(query string, expectedErrMsg string) {
				_, decodeErr := decodeQuery[payloads.BuildpackList](query)
				Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
			},
			Entry("invalid order_by", "order_by=foo", "value must be one of"),
			Entry("per_page is not a number", "per_page=foo", "value must be an integer"),
		)
	})

	Describe("ToMessage", func() {
		var (
			buildpackList payloads.BuildpackList
			message       repositories.ListBuildpacksMessage
		)

		BeforeEach(func() {
			buildpackList = payloads.BuildpackList{
				OrderBy: "created_at",
				Pagination: payloads.Pagination{
					PerPage: "20",
					Page:    "1",
				},
			}
		})

		JustBeforeEach(func() {
			message = buildpackList.ToMessage()
		})

		It("translates to repository message", func() {
			Expect(message).To(Equal(repositories.ListBuildpacksMessage{
				OrderBy: "created_at",
				Pagination: repositories.Pagination{
					Page:    1,
					PerPage: 20,
				},
			}))
		})
	})
})
