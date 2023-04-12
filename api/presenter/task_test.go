package presenter_test

import (
	"encoding/json"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Task", func() {
	var (
		baseURL *url.URL
		output  []byte
		record  repositories.TaskRecord
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		record = repositories.TaskRecord{
			Name:          "task-name",
			GUID:          "task-guid",
			SpaceGUID:     "space-guid",
			Command:       "sleep 10000",
			AppGUID:       "app-guid",
			DropletGUID:   "droplet-guid",
			Labels:        map[string]string{"l": "l1"},
			Annotations:   map[string]string{"a": "a1"},
			SequenceID:    4,
			CreatedAt:     "then",
			UpdatedAt:     "now",
			MemoryMB:      100,
			DiskMB:        200,
			State:         "ok",
			FailureReason: "nope",
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForTask(record, *baseURL)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces expected task json", func() {
		Expect(output).To(MatchJSON(`{
			"name": "task-name",
			"guid": "task-guid",
			"command": "sleep 10000",
			"sequence_id": 4,
			"created_at": "then",
			"updated_at": "now",
			"memory_in_mb": 100,
			"disk_in_mb": 200,
			"droplet_guid": "droplet-guid",
			"state": "ok",
			"metadata": {
				"labels": {"l": "l1"},
				"annotations": {"a": "a1"}
			},
			"relationships": {
				"app": {
					"data": {
						"guid": "app-guid"
					}
				}
			},
			"result": {
				"failure_reason": "nope"
			},
			"links": {
				"self": {
					"href": "https://api.example.org/v3/tasks/task-guid"
				},
				"app": {
					"href": "https://api.example.org/v3/apps/app-guid"
				},
				"droplet": {
					"href": "https://api.example.org/v3/droplets/droplet-guid"
				},
				"cancel": {
					"href": "https://api.example.org/v3/tasks/task-guid/actions/cancel",
					"method": "POST"
				}
			}
		}`))
	})

	When("labels is nil", func() {
		BeforeEach(func() {
			record.Labels = nil
		})

		It("returns an empty slice of labels", func() {
			Expect(output).To(MatchJSONPath("$.metadata.labels", Not(BeNil())))
		})
	})

	When("annotations is nil", func() {
		BeforeEach(func() {
			record.Annotations = nil
		})

		It("returns an empty slice of annotations", func() {
			Expect(output).To(MatchJSONPath("$.metadata.annotations", Not(BeNil())))
		})
	})
})
