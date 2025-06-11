package payloads_test

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Pagination", func() {
	Describe("Validation", func() {
		DescribeTable("valid query",
			func(query string, expectedPagination payloads.Pagination) {
				queryValues, err := url.ParseQuery(query)
				Expect(err).NotTo(HaveOccurred())

				actualPagination := &payloads.Pagination{}
				Expect(actualPagination.DecodeFromURLValues(queryValues)).To(Succeed())

				Expect(*actualPagination).To(Equal(expectedPagination))
			},

			Entry("per_page", "per_page=50", payloads.Pagination{PerPage: "50"}),
			Entry("page=3", "page=3", payloads.Pagination{Page: "3"}),
		)

		DescribeTable("invalid query",
			func(query string, expectedErrMsg string) {
				_, decodeErr := decodeQuery[payloads.AppList](query)
				Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
			},
			Entry("per_page is not a number", "per_page=foo", "value must be an integer"),
			Entry("per_page is zero", "per_page=0", "value 0 is not allowed"),
			Entry("per_page is less than zero", "per_page=-1", "must be no less than 1"),
			Entry("per_page is greater than 5000", "per_page=5001", "must be no greater than 5000"),
			Entry("page is not a number", "page=foo", "value must be an integer"),
			Entry("page is zero", "page=0", "value 0 is not allowed"),
			Entry("page is less than zero", "page=-1", "must be no less than 1"),
		)
	})

	Describe("ToMessage", func() {
		var (
			pagination *payloads.Pagination
			message    repositories.Pagination
		)

		BeforeEach(func() {
			pagination = &payloads.Pagination{
				PerPage: "50",
				Page:    "2",
			}
		})

		JustBeforeEach(func() {
			message = pagination.ToMessage(100)
		})

		It("translates to repository message", func() {
			Expect(message).To(Equal(repositories.Pagination{
				Page:    2,
				PerPage: 50,
			}))
		})

		When("per_page is set", func() {
			BeforeEach(func() {
				pagination.PerPage = "10"
			})

			It("uses the provided value", func() {
				Expect(message.PerPage).To(Equal(10))
			})
		})

		When("page is set", func() {
			BeforeEach(func() {
				pagination.Page = "5"
			})

			It("uses the provided value", func() {
				Expect(message.Page).To(Equal(5))
			})
		})
	})
})
