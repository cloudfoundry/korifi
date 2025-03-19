package presenter_test

import (
	"encoding/json"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("", func() {
	var (
		baseURL *url.URL
		output  []byte
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("JobFromGUID", func() {
		var (
			job   presenter.Job
			match bool
			guid  string
		)

		BeforeEach(func() {
			guid = "resource.operation~guid"
		})

		JustBeforeEach(func() {
			job, match = presenter.JobFromGUID(guid)
		})

		It("parses a job GUID into a Job struct", func() {
			Expect(match).To(BeTrue())
			Expect(job).To(Equal(presenter.Job{
				GUID:         "resource.operation~guid",
				Type:         "resource.operation",
				ResourceGUID: "guid",
				ResourceType: "resource",
			}))
		})
	})

	Describe("ForManifestApplyJob", func() {
		JustBeforeEach(func() {
			response := presenter.ForManifestApplyJob(presenter.Job{
				GUID:         "the-job-guid",
				Type:         presenter.SpaceApplyManifestOperation,
				ResourceGUID: "the-space-guid",
			}, *baseURL)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("renders the job", func() {
			Expect(output).To(MatchJSON(`{
				"errors": [],
				"guid": "the-job-guid",
				"links": {
					"self": {
						"href": "https://api.example.org/v3/jobs/the-job-guid"
					},
					"space": {
						"href": "https://api.example.org/v3/spaces/the-space-guid"
					}
				},
				"operation": "space.apply_manifest",
				"state": "COMPLETE"
			}`))
		})
	})

	Describe("ForJob", func() {
		var (
			job    presenter.Job
			errors []presenter.JobResponseError
			state  repositories.ResourceState
		)

		BeforeEach(func() {
			job = presenter.Job{
				GUID: "the-job-guid",
				Type: "the.operation",
			}
			errors = []presenter.JobResponseError{}
			state = repositories.ResourceStateReady
		})

		JustBeforeEach(func() {
			response := presenter.ForJob(job, errors, state, *baseURL)

			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("renders the job", func() {
			Expect(output).To(MatchJSON(`{
				"errors": [],
				"guid": "the-job-guid",
				"links": {
					"self": {
						"href": "https://api.example.org/v3/jobs/the-job-guid"
					}
				},
				"operation": "the.operation",
				"state": "COMPLETE"
			}`))
		})

		When("there are errors", func() {
			BeforeEach(func() {
				errors = []presenter.JobResponseError{{
					Detail: "error detail",
					Title:  "CF-JobErrorTitle",
					Code:   12345,
				}}
			})

			It("renders them in the job", func() {
				Expect(output).To(matchers.MatchJSONPath("$.errors[0]", MatchAllKeys(Keys{
					"detail": Equal("error detail"),
					"title":  Equal("CF-JobErrorTitle"),
					"code":   BeEquivalentTo(12345),
				})))
			})

			It("renders the job as FAILED", func() {
				Expect(output).To(matchers.MatchJSONPath("$.state", Equal("FAILED")))
			})
		})

		When("the job resource is not ready", func() {
			BeforeEach(func() {
				state = repositories.ResourceStateUnknown
			})

			It("renders the job as PROCESSING", func() {
				Expect(output).To(matchers.MatchJSONPath("$.state", Equal("PROCESSING")))
			})
		})

		When("the job refers to a service instance that is not ready", func() {
			BeforeEach(func() {
				job.ResourceType = presenter.ManagedServiceInstanceResourceType
				state = repositories.ResourceStateUnknown
			})

			It("renders the job as POLLING", func() {
				Expect(output).To(matchers.MatchJSONPath("$.state", Equal("POLLING")))
			})
		})

		When("the job refers to a service binding that is not ready", func() {
			BeforeEach(func() {
				job.ResourceType = presenter.ManagedServiceBindingResourceType
				state = repositories.ResourceStateUnknown
			})

			It("renders the job as POLLING", func() {
				Expect(output).To(matchers.MatchJSONPath("$.state", Equal("POLLING")))
			})
		})
	})
})
