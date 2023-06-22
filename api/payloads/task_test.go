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
	var (
		createPayload payloads.TaskCreate
		taskCreate    *payloads.TaskCreate
		validatorErr  error
	)

	BeforeEach(func() {
		taskCreate = new(payloads.TaskCreate)
		createPayload = payloads.TaskCreate{
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

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(createPayload), taskCreate)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(taskCreate).To(gstruct.PointTo(Equal(createPayload)))
	})

	When("no command is set", func() {
		BeforeEach(func() {
			createPayload.Command = ""
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Command is a required field")
		})
	})

	When("metadata.labels contains an invalid key", func() {
		BeforeEach(func() {
			createPayload.Metadata = payloads.Metadata{
				Labels: map[string]string{
					"foo.cloudfoundry.org/bar": "jim",
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
		})
	})

	When("metadata.annotations contains an invalid key", func() {
		BeforeEach(func() {
			createPayload.Metadata = payloads.Metadata{
				Annotations: map[string]string{
					"foo.cloudfoundry.org/bar": "jim",
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
		})
	})

	Context("ToMessage()", func() {
		It("converts to repo message correctly", func() {
			msg := taskCreate.ToMessage(repositories.AppRecord{GUID: "appGUID", SpaceGUID: "spaceGUID"})
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
	var (
		updatePayload payloads.TaskUpdate
		taskUpdate    *payloads.TaskUpdate
		validatorErr  error
	)

	BeforeEach(func() {
		taskUpdate = new(payloads.TaskUpdate)
		updatePayload = payloads.TaskUpdate{
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

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(updatePayload), taskUpdate)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(taskUpdate).To(gstruct.PointTo(Equal(updatePayload)))
	})

	When("metadata.labels contains an invalid key", func() {
		BeforeEach(func() {
			updatePayload.Metadata = payloads.MetadataPatch{
				Labels: map[string]*string{
					"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
		})
	})

	When("metadata.annotations contains an invalid key", func() {
		BeforeEach(func() {
			updatePayload.Metadata = payloads.MetadataPatch{
				Annotations: map[string]*string{
					"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
		})
	})

	Context("toMessage()", func() {
		It("converts to repo message correctly", func() {
			msg := taskUpdate.ToMessage("taskGUID", "spaceGUID")
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
