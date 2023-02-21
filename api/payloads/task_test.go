package payloads_test

import (
	"bytes"
	"code.cloudfoundry.org/korifi/tools"
	"encoding/json"
	"github.com/onsi/gomega/gstruct"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TaskList", func() {
	DescribeTable("DecodeFromURLValues",
		func(query string, taskList payloads.TaskList, err string) {
			actualTaskList := payloads.TaskList{}
			values, parseErr := url.ParseQuery(query)
			Expect(parseErr).NotTo(HaveOccurred())

			decodeErr := actualTaskList.DecodeFromURLValues(values)

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
		body, err := json.Marshal(updatePayload)
		Expect(err).NotTo(HaveOccurred())

		req, err := http.NewRequest("", "", bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())

		validatorErr = validator.DecodeAndValidateJSONPayload(req, taskUpdate)
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
