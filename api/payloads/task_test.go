package payloads_test

import (
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
