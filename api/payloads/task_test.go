package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/onsi/gomega/gstruct"

	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TaskList", func() {
	DescribeTable("valid query",
		func(query string, expectedTaskList payloads.TaskList) {
			actualTaskList, decodeErr := decodeQuery[payloads.TaskList](query)

			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(*actualTaskList).To(Equal(expectedTaskList))
		},
		Entry("order_by created_at", "order_by=created_at", payloads.TaskList{OrderBy: "created_at"}),
		Entry("order_by -created_at", "order_by=-created_at", payloads.TaskList{OrderBy: "-created_at"}),
		Entry("order_by updated_at", "order_by=updated_at", payloads.TaskList{OrderBy: "updated_at"}),
		Entry("order_by -updated_at", "order_by=-updated_at", payloads.TaskList{OrderBy: "-updated_at"}),
		Entry("pagination", "page=3", payloads.TaskList{Pagination: payloads.Pagination{Page: "3"}}),
	)

	DescribeTable("invalid query",
		func(query string, expectedErrMsg string) {
			_, decodeErr := decodeQuery[payloads.TaskList](query)
			Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
		},
		Entry("invalid order_by", "order_by=foo", "value must be one of"),
		Entry("invalid parameter", "foo=bar", "unsupported query parameter: foo"),
		Entry("invalid pagination", "per_page=foo", "value must be an integer"),
	)

	Describe("ToMessage", func() {
		It("translates to repo message", func() {
			taskList := payloads.TaskList{
				OrderBy: "created_at",
				Pagination: payloads.Pagination{
					PerPage: "3",
					Page:    "2",
				},
			}
			Expect(taskList.ToMessage()).To(Equal(repositories.ListTasksMessage{
				OrderBy: "created_at",
				Pagination: repositories.Pagination{
					Page:    2,
					PerPage: 3,
				},
			}))
		})
	})
})

var _ = Describe("AppTaskList", func() {
	DescribeTable("valid query",
		func(query string, expectedAppTaskList payloads.AppTaskList) {
			actualAppTaskList, decodeErr := decodeQuery[payloads.AppTaskList](query)

			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(*actualAppTaskList).To(Equal(expectedAppTaskList))
		},

		Entry("missing sequence_ids", "", payloads.AppTaskList{}),
		Entry("empty sequence_ids", "sequence_ids=", payloads.AppTaskList{}),
		Entry("valid sequence_ids", "sequence_ids=1,2,3", payloads.AppTaskList{SequenceIDs: []int64{1, 2, 3}}),
		Entry("empty sequence_id", "sequence_ids=1,,3", payloads.AppTaskList{SequenceIDs: []int64{1, 3}}),
		Entry("order_by created_at", "order_by=created_at", payloads.AppTaskList{OrderBy: "created_at"}),
		Entry("order_by -created_at", "order_by=-created_at", payloads.AppTaskList{OrderBy: "-created_at"}),
		Entry("order_by updated_at", "order_by=updated_at", payloads.AppTaskList{OrderBy: "updated_at"}),
		Entry("order_by -updated_at", "order_by=-updated_at", payloads.AppTaskList{OrderBy: "-updated_at"}),
		Entry("pagination", "page=3", payloads.AppTaskList{Pagination: payloads.Pagination{Page: "3"}}),
	)

	DescribeTable("invalid query",
		func(query string, expectedErrMsg string) {
			_, decodeErr := decodeQuery[payloads.AppTaskList](query)
			Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
		},
		Entry("invalid sequence_ids", "sequence_ids=1,two,3", "invalid syntax"),
		Entry("invalid parameter", "foo=bar", "unsupported query parameter: foo"),
		Entry("invalid order_by", "order_by=foo", "value must be one of"),
		Entry("invalid pagination", "per_page=foo", "value must be an integer"),
	)

	Describe("ToMessage", func() {
		It("translates to repo message", func() {
			appTaskList := payloads.AppTaskList{
				SequenceIDs: []int64{1, 2, 3},
				OrderBy:     "created_at",
				Pagination: payloads.Pagination{
					PerPage: "3",
					Page:    "2",
				},
			}
			Expect(appTaskList.ToMessage()).To(Equal(repositories.ListTasksMessage{
				SequenceIDs: []int64{1, 2, 3},
				OrderBy:     "created_at",
				Pagination: repositories.Pagination{
					Page:    2,
					PerPage: 3,
				},
			}))
		})
	})
})

var _ = Describe("TaskCreate", func() {
	var payload payloads.TaskCreate

	BeforeEach(func() {
		payload = payloads.TaskCreate{
			Command: "sleep 9000",
			Metadata: payloads.Metadata{
				Labels: map[string]string{
					"foo": "bar",
					"bar": "baz",
				},
				Annotations: map[string]string{
					"example.org/jim": "hello",
				},
			},
		}
	})

	Describe("Validate", func() {
		var (
			decodedPayload *payloads.TaskCreate
			validatorErr   error
		)

		JustBeforeEach(func() {
			decodedPayload = new(payloads.TaskCreate)
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(payload), decodedPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedPayload).To(gstruct.PointTo(Equal(payload)))
		})

		When("no command is set", func() {
			BeforeEach(func() {
				payload.Command = ""
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "command cannot be blank")
			})
		})

		When("metadata is invalid", func() {
			BeforeEach(func() {
				payload.Metadata = payloads.Metadata{
					Labels: map[string]string{
						"foo.cloudfoundry.org/bar": "jim",
					},
				}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "label/annotation key cannot use the cloudfoundry.org domain")
			})
		})
	})

	Describe("ToMessage()", func() {
		It("converts to repo message correctly", func() {
			msg := payload.ToMessage(repositories.AppRecord{GUID: "appGUID", SpaceGUID: "spaceGUID"})
			Expect(msg.AppGUID).To(Equal("appGUID"))
			Expect(msg.SpaceGUID).To(Equal("spaceGUID"))
			Expect(msg.Metadata.Labels).To(Equal(map[string]string{
				"foo": "bar",
				"bar": "baz",
			}))
			Expect(msg.Metadata.Annotations).To(Equal(map[string]string{
				"example.org/jim": "hello",
			}))
		})
	})
})

var _ = Describe("TaskUpdate", func() {
	var payload payloads.TaskUpdate

	BeforeEach(func() {
		payload = payloads.TaskUpdate{
			Metadata: payloads.MetadataPatch{
				Labels: map[string]*string{
					"foo": tools.PtrTo("bar"),
					"bar": nil,
				},
				Annotations: map[string]*string{
					"example.org/jim": tools.PtrTo("hello"),
				},
			},
		}
	})

	Describe("Validate", func() {
		var (
			decodedPayload *payloads.TaskUpdate
			validatorErr   error
		)

		JustBeforeEach(func() {
			decodedPayload = new(payloads.TaskUpdate)
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(payload), decodedPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedPayload).To(gstruct.PointTo(Equal(payload)))
		})

		When("metadata is invalid", func() {
			BeforeEach(func() {
				payload.Metadata = payloads.MetadataPatch{
					Labels: map[string]*string{
						"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
					},
				}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "label/annotation key cannot use the cloudfoundry.org domain")
			})
		})
	})

	Describe("ToMessage()", func() {
		It("converts to repo message correctly", func() {
			msg := payload.ToMessage("taskGUID", "spaceGUID")
			Expect(msg.TaskGUID).To(Equal("taskGUID"))
			Expect(msg.SpaceGUID).To(Equal("spaceGUID"))
			Expect(msg.MetadataPatch.Labels).To(Equal(map[string]*string{
				"foo": tools.PtrTo("bar"),
				"bar": nil,
			}))
			Expect(msg.MetadataPatch.Annotations).To(Equal(map[string]*string{
				"example.org/jim": tools.PtrTo("hello"),
			}))
		})
	})
})
