package presenter_test

import (
	"encoding/json"
	"time"

	"code.cloudfoundry.org/korifi/api/handlers/stats"
	"code.cloudfoundry.org/korifi/api/presenter"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Process Stats", func() {
	var (
		output         []byte
		gauges         []stats.ProcessGauges
		instancesState []stats.ProcessInstanceState
	)

	BeforeEach(func() {
		gauges = []stats.ProcessGauges{{
			Index:     0,
			CPU:       tools.PtrTo(500.0),
			Mem:       tools.PtrTo(int64(512)),
			Disk:      tools.PtrTo(int64(256)),
			MemQuota:  tools.PtrTo(int64(1024)),
			DiskQuota: tools.PtrTo(int64(2048)),
		}, {
			Index:     1,
			CPU:       tools.PtrTo(501.0),
			Mem:       tools.PtrTo(int64(513)),
			Disk:      tools.PtrTo(int64(257)),
			MemQuota:  tools.PtrTo(int64(1025)),
			DiskQuota: tools.PtrTo(int64(2049)),
		}}

		instancesState = []stats.ProcessInstanceState{
			{
				ID:    0,
				Type:  "web",
				State: korifiv1alpha1.InstanceStateRunning,
			},
			{
				ID:    1,
				Type:  "web",
				State: korifiv1alpha1.InstanceStateDown,
			},
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForProcessStats(gauges, instancesState, time.UnixMilli(2000).UTC())
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
					"mem_quota": 1024,
					"disk_quota": 2048,
					"usage": {
						"time": "1970-01-01T00:00:02Z",
						"cpu": 500,
						"mem": 512,
						"disk": 256
					}
				},
				{
					"type": "web",
					"index": 1,
					"state": "DOWN",
					"mem_quota": 1025,
					"disk_quota": 2049,
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

	When("there are gauges for instance that is not available", func() {
		BeforeEach(func() {
			instancesState = []stats.ProcessInstanceState{
				{
					ID:    1,
					Type:  "web",
					State: korifiv1alpha1.InstanceStateDown,
				},
			}
		})

		It("filters gauges for not available instances", func() {
			Expect(output).To(MatchJSON(`{
				"resources": [
					{
						"type": "web",
						"index": 1,
						"state": "DOWN",
						"mem_quota": 1025,
						"disk_quota": 2049,
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
	})

	When("there are no gauges for an instance", func() {
		BeforeEach(func() {
			gauges = []stats.ProcessGauges{{
				Index:     1,
				CPU:       tools.PtrTo(501.0),
				Mem:       tools.PtrTo(int64(513)),
				Disk:      tools.PtrTo(int64(257)),
				MemQuota:  tools.PtrTo(int64(1025)),
				DiskQuota: tools.PtrTo(int64(2049)),
			}}
		})

		It("returns zero values for instances without gauges", func() {
			Expect(output).To(MatchJSON(`{
				"resources": [
					{
						"type": "web",
						"index": 0,
						"state": "RUNNING"
					},
					{
						"type": "web",
						"index": 1,
						"state": "DOWN",
						"mem_quota": 1025,
						"disk_quota": 2049,
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
	})
})
