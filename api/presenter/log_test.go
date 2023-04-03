package presenter_test

import (
	"encoding/json"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LogCache", func() {
	var (
		output  []byte
		records []repositories.LogRecord
	)

	BeforeEach(func() {
		records = []repositories.LogRecord{
			{
				Message:   "message-1",
				Timestamp: 123,
				Tags: map[string]string{
					"foo": "bar",
				},
			},
			{
				Message:   "message-2",
				Timestamp: 456,
			},
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForLogs(records)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces expected logs json", func() {
		Expect(output).To(MatchJSON(`{
			"envelopes": {
				"batch": [
					{
						"timestamp": 123,
						"log": {
							"payload": "bWVzc2FnZS0x",
							"type": 0
						},
						"tags": {
							"foo": "bar"
						}
					},
					{
						"timestamp": 456,
						"log": {
							"payload": "bWVzc2FnZS0y",
							"type": 0
						}
					}
				]
			}
		}`))
	})
})
