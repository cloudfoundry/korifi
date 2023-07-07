package presenter_test

import (
	"encoding/json"
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Build", func() {
	var (
		baseURL *url.URL
		output  []byte
		record  repositories.BuildRecord
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		record = repositories.BuildRecord{
			GUID:            "build-guid",
			State:           "STAGING",
			CreatedAt:       time.UnixMilli(1000),
			UpdatedAt:       tools.PtrTo(time.UnixMilli(2000)),
			StagingMemoryMB: 128,
			StagingDiskMB:   512,
			Lifecycle: repositories.Lifecycle{
				Type: "buildpack",
				Data: repositories.LifecycleData{
					Buildpacks: []string{"foo"},
					Stack:      "fdsa",
				},
			},
			PackageGUID: "package-guid",
			AppGUID:     "app-guid",
			Labels: map[string]string{
				"label-key": "label-val",
			},
			Annotations: map[string]string{
				"annotation-key": "annotation-val",
			},
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForBuild(record, *baseURL)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces expected build json", func() {
		Expect(output).To(MatchJSON(`{
			"guid": "build-guid",
			"created_at": "1970-01-01T00:00:01Z",
			"updated_at": "1970-01-01T00:00:02Z",
			"created_by": {},
			"state": "STAGING",
			"staging_memory_in_mb": 128,
			"staging_disk_in_mb": 512,
			"error": null,
			"lifecycle": {
				"type": "buildpack",
				"data": {
					"buildpacks": [
						"foo"
					],
					"stack": "fdsa"
				}
			},
			"package": {
				"guid": "package-guid"
			},
			"droplet": null,
			"relationships": {
				"app": {
					"data": {
						"guid": "app-guid"
					}
				}
			},
			"metadata": {
				"labels": {
					"label-key": "label-val"
				},
				"annotations": {
					"annotation-key": "annotation-val"
				}
			},
			"links": {
				"self": {
					"href": "https://api.example.org/v3/builds/build-guid"
				},
				"app": {
					"href": "https://api.example.org/v3/apps/app-guid"
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

	When("droplet is set", func() {
		BeforeEach(func() {
			record.DropletGUID = "droplet-guid"
		})

		It("includes droplet and link/droplet", func() {
			Expect(output).To(MatchJSONPath("$.droplet.guid", "droplet-guid"))
			Expect(output).To(MatchJSONPath("$.links.droplet.href", HaveSuffix("droplet-guid")))
		})
	})

	When("staging error is set", func() {
		BeforeEach(func() {
			record.StagingErrorMsg = "oops"
		})

		It("includes the error message", func() {
			Expect(output).To(MatchJSONPath("$.error", "oops"))
		})
	})
})
