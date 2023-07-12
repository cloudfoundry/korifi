package handlers_test

import (
	"fmt"
	"net/http"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Job", func() {
	var (
		handler       *handlers.Job
		deletionRepos map[string]handlers.DeletionRepository
		jobGUID       string
		req           *http.Request
	)

	BeforeEach(func() {
		deletionRepos = map[string]handlers.DeletionRepository{}
	})

	JustBeforeEach(func() {
		handler = handlers.NewJob(*serverURL, deletionRepos, 0)
		routerBuilder.LoadRoutes(handler)

		var err error
		req, err = http.NewRequestWithContext(ctx, "GET", "/v3/jobs/"+jobGUID, nil)
		Expect(err).NotTo(HaveOccurred())

		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("GET /v3/jobs/space.apply_manifest", func() {
		BeforeEach(func() {
			jobGUID = "space.apply_manifest~cf-space-guid"
		})

		It("returns a complete status", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", jobGUID),
				MatchJSONPath("$.links.self.href", defaultServerURL+"/v3/jobs/"+jobGUID),
				MatchJSONPath("$.operation", "space.apply_manifest"),
				MatchJSONPath("$.state", "COMPLETE"),
				MatchJSONPath("$.links.space.href", defaultServerURL+"/v3/spaces/cf-space-guid"),
			)))
		})
	})

	Describe("GET /v3/jobs/*", func() {
		var deletionRepo *fake.DeletionRepository

		BeforeEach(func() {
			deletionRepo = new(fake.DeletionRepository)
			deletionRepo.GetDeletedAtReturns(tools.PtrTo(time.Now()), nil)
			deletionRepos["testing.delete"] = deletionRepo

			jobGUID = "testing.delete~my-resource-guid"
		})

		It("returns a processing status", func() {
			Expect(deletionRepo.GetDeletedAtCallCount()).To(Equal(1))
			_, actualAuthInfo, actualResourceGUID := deletionRepo.GetDeletedAtArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualResourceGUID).To(Equal("my-resource-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", jobGUID),
				MatchJSONPath("$.links.self.href", defaultServerURL+"/v3/jobs/"+jobGUID),
				MatchJSONPath("$.operation", "testing.delete"),
				MatchJSONPath("$.state", "PROCESSING"),
				MatchJSONPath("$.errors", BeEmpty()),
			)))
		})

		When("the resource does not exist", func() {
			BeforeEach(func() {
				deletionRepo.GetDeletedAtReturns(nil, fmt.Errorf("wrapped error: %w", apierrors.NewNotFoundError(nil, "foo")))
			})

			It("returns a complete status", func() {
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.state", "COMPLETE"),
					MatchJSONPath("$.errors", BeEmpty()),
				)))
			})
		})

		When("the resource deletion times out", func() {
			BeforeEach(func() {
				deletionRepo.GetDeletedAtReturns(tools.PtrTo(time.Now().Add(-180*time.Second)), nil)
			})

			It("returns a failed status", func() {
				Expect(deletionRepo.GetDeletedAtCallCount()).To(Equal(1))
				_, actualAuthInfo, actualResourceGUID := deletionRepo.GetDeletedAtArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(actualResourceGUID).To(Equal("my-resource-guid"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.state", "FAILED"),
					MatchJSONPath("$.errors", ConsistOf(map[string]interface{}{
						"code":   float64(10008),
						"detail": "Testing deletion timed out, check the remaining \"my-resource-guid\" resource",
						"title":  "CF-UnprocessableEntity",
					})),
				)))
			})
		})

		When("the user does not have permission to see the resource", func() {
			BeforeEach(func() {
				deletionRepo.GetDeletedAtReturns(nil, fmt.Errorf("wrapped err: %w", apierrors.NewForbiddenError(nil, "foo")))
			})

			It("returns a complete status", func() {
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.state", "COMPLETE"),
					MatchJSONPath("$.errors", BeEmpty()),
				)))
			})
		})

		When("the resource has not been marked for deletion", func() {
			BeforeEach(func() {
				deletionRepo.GetDeletedAtReturns(nil, nil)
			})

			It("returns a not found error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.errors[0].code", float64(10010)),
					MatchJSONPath("$.errors[0].detail", "Job not found. Ensure it exists and you have access to it."),
					MatchJSONPath("$.errors[0].title", "CF-ResourceNotFound"),
				)))
			})
		})

		When("the job guid is invalid", func() {
			BeforeEach(func() {
				jobGUID = "job.operation;some-resource-guid"
			})

			It("returns an error", func() {
				expectNotFoundError("Job")
			})
		})

		When("there is no deletion repository registered for the operation", func() {
			BeforeEach(func() {
				deletionRepos = map[string]handlers.DeletionRepository{}
			})

			It("returns an error", func() {
				expectNotFoundError("Job")
			})
		})
	})
})
