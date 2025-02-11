package presenter_test

import (
	"encoding/json"
	"time"

	"code.cloudfoundry.org/korifi/api/actions"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Process Stats", func() {
	var (
		output  []byte
		records []actions.PodStatsRecord
	)

	BeforeEach(func() {
		var err error
		Expect(err).NotTo(HaveOccurred())
		records = []actions.PodStatsRecord{
			{
				ProcessType: "web",
				Index:       0,
				State:       "RUNNING",
				Usage: actions.Usage{
					Timestamp: tools.PtrTo(time.UnixMilli(1000).UTC()),
					CPU:       tools.PtrTo(500.0),
					Mem:       tools.PtrTo(int64(512)),
					Disk:      tools.PtrTo(int64(256)),
				},
				MemQuota:  tools.PtrTo(int64(1024)),
				DiskQuota: tools.PtrTo(int64(2048)),
			},
			{
				ProcessType: "web",
				Index:       1,
				State:       "RUNNING",
				Usage: actions.Usage{
					Timestamp: tools.PtrTo(time.UnixMilli(2000).UTC()),
					CPU:       tools.PtrTo(501.0),
					Mem:       tools.PtrTo(int64(513)),
					Disk:      tools.PtrTo(int64(257)),
				},
				MemQuota:  tools.PtrTo(int64(1024)),
				DiskQuota: tools.PtrTo(int64(2048)),
			},
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForProcessStats(records)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces expected process stats json", func() {
		Expect(output).To(MatchJSON(`{
			"resources": [
				{
					"type": "web",
					"index": 0,
					"state": "RUNNING",
					"host": null,
					"uptime": null,
					"mem_quota": 1024,
					"disk_quota": 2048,
					"fds_quota": null,
					"isolation_segment": null,
					"details": null,
					"instance_ports": [],
					"usage": {
						"time": "1970-01-01T00:00:01Z",
						"cpu": 500,
						"mem": 512,
						"disk": 256
					}
				},
				{
					"type": "web",
					"index": 1,
					"state": "RUNNING",
					"host": null,
					"uptime": null,
					"mem_quota": 1024,
					"disk_quota": 2048,
					"fds_quota": null,
					"isolation_segment": null,
					"details": null,
					"instance_ports": [],
					"usage": {
						"time": "1970-01-01T00:00:02Z",
						"cpu": 501,
						"mem": 513,
						"disk": 257
					}
				}
			]
		}`))
	})

	When("process is down", func() {
		BeforeEach(func() {
			records[0].State = "DOWN"
			records[1].State = "DOWN"
		})

		It("omits nil instance ports", func() {
			Expect(output).ToNot(ContainSubstring("instance_ports"))
		})
	})
})
