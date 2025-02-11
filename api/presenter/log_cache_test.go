package presenter_test

import (
	"encoding/json"
	"time"

	"code.cloudfoundry.org/korifi/api/actions"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ForLogs", func() {
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

var _ = Describe("ForStats", func() {
	var (
		output []byte
		app    repositories.AppRecord
		stats  []actions.PodStatsRecord
	)

	BeforeEach(func() {
		app = repositories.AppRecord{
			Name:      "my-app",
			GUID:      "app-guid",
			SpaceGUID: "space-guid",
		}

		stats = []actions.PodStatsRecord{{
			ProcessType: "web",
			ProcessGUID: "process-guid",
			Index:       1,
			Usage: actions.Usage{
				Timestamp: tools.PtrTo(time.UnixMilli(1000).UTC()),
				CPU:       tools.PtrTo(1e-05),
				Mem:       tools.PtrTo[int64](2),
				Disk:      tools.PtrTo[int64](3),
			},
			MemQuota:  tools.PtrTo[int64](4),
			DiskQuota: tools.PtrTo[int64](5),
		}}
	})

	JustBeforeEach(func() {
		response := presenter.ForStats(app, stats)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces expected stats json", func() {
		Expect(output).To(MatchJSON(`{
		  "envelopes": {
			"batch": [
			  {
				"timestamp": 1,
				"tags": {
				  "app_id": "app-guid",
				  "app_name": "my-app",
				  "instance_id": "1",
				  "process_type": "web",
				  "process_id": "process-guid",
				  "source_id": "app-guid",
				  "space_id": "space-guid"
				},
				"gauge": {
				  "metrics": {
					"cpu": {
					  "unit": "percentage",
					  "value": 0.00001
					},
					"disk": {
					  "unit": "bytes",
					  "value": 3
					},
					"disk_quota": {
					  "unit": "bytes",
					  "value": 5
					},
					"memory": {
					  "unit": "bytes",
					  "value": 2
					},
					"memory_quota": {
					  "unit": "bytes",
					  "value": 4
					}
				  }
				}
			  }
			]
		  }
		}`))
	})
})
