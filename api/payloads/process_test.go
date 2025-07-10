package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("ProcessList", func() {
	Describe("Validation", func() {
		DescribeTable("valid query",
			func(query string, expectedProcessList payloads.ProcessList) {
				actualProcessList, decodeErr := decodeQuery[payloads.ProcessList](query)

				Expect(decodeErr).NotTo(HaveOccurred())
				Expect(*actualProcessList).To(Equal(expectedProcessList))
			},

			Entry("app_guids", "app_guids=guid1,guid2", payloads.ProcessList{AppGUIDs: "guid1,guid2"}),
			Entry("created_at", "order_by=created_at", payloads.ProcessList{OrderBy: "created_at"}),
			Entry("-created_at", "order_by=-created_at", payloads.ProcessList{OrderBy: "-created_at"}),
			Entry("updated_at", "order_by=updated_at", payloads.ProcessList{OrderBy: "updated_at"}),
			Entry("-updated_at", "order_by=-updated_at", payloads.ProcessList{OrderBy: "-updated_at"}),
			Entry("page=3", "page=3", payloads.ProcessList{Pagination: payloads.Pagination{Page: "3"}}),
		)

		DescribeTable("invalid query",
			func(query string, expectedErrMsg string) {
				_, decodeErr := decodeQuery[payloads.ProcessList](query)
				Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
			},
			Entry("invalid order_by", "order_by=foo", "value must be one of"),
			Entry("per_page is not a number", "per_page=foo", "value must be an integer"),
		)
	})

	Describe("ToMessage", func() {
		var (
			processList payloads.ProcessList
			message     repositories.ListProcessesMessage
		)

		BeforeEach(func() {
			processList = payloads.ProcessList{
				AppGUIDs: "ag1,ag2",
				Pagination: payloads.Pagination{
					PerPage: "10",
					Page:    "4",
				},
			}
		})

		JustBeforeEach(func() {
			message = processList.ToMessage()
		})

		It("translates to repository message", func() {
			Expect(message).To(Equal(repositories.ListProcessesMessage{
				AppGUIDs: []string{"ag1", "ag2"},
				Pagination: repositories.Pagination{
					PerPage: 10,
					Page:    4,
				},
			}))
		})
	})
})

var _ = Describe("Process payload validation", func() {
	var validatorErr error

	Describe("ProcessScale", func() {
		var (
			payload        payloads.ProcessScale
			decodedPayload *payloads.ProcessScale
		)

		BeforeEach(func() {
			payload = payloads.ProcessScale{
				Instances: tools.PtrTo[int32](1),
				MemoryMB:  tools.PtrTo[int64](2),
				DiskMB:    tools.PtrTo[int64](3),
			}

			decodedPayload = new(payloads.ProcessScale)
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(payload), decodedPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedPayload).To(gstruct.PointTo(Equal(payload)))
		})

		When("instances is negative", func() {
			BeforeEach(func() {
				payload.Instances = tools.PtrTo[int32](-1)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "instances must be 0 or greater")
			})
		})

		When("memory is negative", func() {
			BeforeEach(func() {
				payload.MemoryMB = tools.PtrTo[int64](-1)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "memory_in_mb must be greater than 0")
			})
		})

		When("disk is negative", func() {
			BeforeEach(func() {
				payload.DiskMB = tools.PtrTo[int64](-1)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "disk_in_mb must be greater than 0")
			})
		})
	})
})
