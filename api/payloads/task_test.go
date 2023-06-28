package payloads_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/onsi/gomega/gstruct"

	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TaskList", func() {
	DescribeTable("decodes from url values",
		func(query string, taskList payloads.TaskList, err string) {
			actualTaskList := payloads.TaskList{}
			req, reqErr := http.NewRequest("GET", "http://foo.com/?"+query, nil)
			Expect(reqErr).NotTo(HaveOccurred())

			decodeErr := validator.DecodeAndValidateURLValues(req, &actualTaskList)

			if err == "" {
				Expect(decodeErr).NotTo(HaveOccurred())
				Expect(actualTaskList).To(Equal(taskList))
			} else {
				Expect(decodeErr).To(MatchError(ContainSubstring(err)))
			}
		},
		Entry("valid sequence_ids", "sequence_ids=1,2,3", payloads.TaskList{SequenceIDs: []int64{1, 2, 3}}, ""),
		Entry("missing sequence_ids", "", payloads.TaskList{}, ""),
		Entry("empty sequence_ids", "sequence_ids=", payloads.TaskList{}, ""),
		Entry("empty sequence_id", "sequence_ids=1,,3", payloads.TaskList{SequenceIDs: []int64{1, 3}}, ""),
		Entry("invalid sequence_ids", "sequence_ids=1,two,3", payloads.TaskList{}, "invalid syntax"),
	)
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
