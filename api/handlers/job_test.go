package handlers_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Job", func() {
	Describe("GET /v3/jobs endpoint", func() {
		var (
			spaceGUID string
			jobGUID   string
			req       *http.Request
			orgRepo   *fake.OrgRepository
		)

		BeforeEach(func() {
			spaceGUID = uuid.NewString()

			orgRepo = new(fake.OrgRepository)
			apiHandler := handlers.NewJob(*serverURL, orgRepo)
			routerBuilder.LoadRoutes(apiHandler)
		})

		JustBeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/jobs/"+jobGUID, nil)
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		Describe("apply manifest jobs", func() {
			BeforeEach(func() {
				jobGUID = "space.apply_manifest~" + spaceGUID
			})

			It("returns the job", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.guid", jobGUID),
					MatchJSONPath("$.links.space.href", defaultServerURL+"/v3/spaces/"+spaceGUID),
					MatchJSONPath("$.operation", "space.apply_manifest"),
				)))
			})
		})

		DescribeTable("delete jobs", func(operation, resourceGUID string) {
			guid := operation + "~" + resourceGUID
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/jobs/"+guid, nil)
			Expect(err).NotTo(HaveOccurred())

			rr = httptest.NewRecorder()
			routerBuilder.Build().ServeHTTP(rr, req)

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", guid),
				MatchJSONPath("$.links.self.href", defaultServerURL+"/v3/jobs/"+guid),
				MatchJSONPath("$.operation", operation),
			)))
		},
			Entry("app delete", "app.delete", "cf-app-guid"),
			Entry("space delete", "space.delete", "cf-space-guid"),
			Entry("route delete", "route.delete", "cf-route-guid"),
			Entry("domain delete", "domain.delete", "cf-domain-guid"),
			Entry("role delete", "role.delete", "cf-role-guid"),
		)

		When("the job guid provided does not have the expected delimiter", func() {
			BeforeEach(func() {
				jobGUID = "job.operation;some-resource-guid"
			})

			It("returns an error", func() {
				expectNotFoundError("Job")
			})
		})

		Describe("org delete", func() {
			const (
				operation    = "org.delete"
				resourceGUID = "cf-org-guid"
			)

			BeforeEach(func() {
				jobGUID = operation + "~" + resourceGUID
			})

			When("the org deletion is in progress", func() {
				BeforeEach(func() {
					orgRepo.GetOrgReturns(repositories.OrgRecord{
						GUID:      "cf-org-guid",
						DeletedAt: time.Now().Format(time.RFC3339Nano),
					}, nil)
				})

				It("returns a processing status", func() {
					Expect(rr).To(HaveHTTPBody(SatisfyAll(
						MatchJSONPath("$.guid", jobGUID),
						MatchJSONPath("$.links.self.href", defaultServerURL+"/v3/jobs/"+jobGUID),
						MatchJSONPath("$.operation", operation),
						MatchJSONPath("$.state", "PROCESSING"),
						MatchJSONPath("$.errors", BeEmpty()),
					)))
				})
			})

			When("the org does not exist", func() {
				BeforeEach(func() {
					orgRepo.GetOrgReturns(repositories.OrgRecord{}, apierrors.NewNotFoundError(nil, repositories.OrgResourceType))
				})

				It("returns a complete status", func() {
					Expect(rr).To(HaveHTTPBody(SatisfyAll(
						MatchJSONPath("$.guid", jobGUID),
						MatchJSONPath("$.links.self.href", defaultServerURL+"/v3/jobs/"+jobGUID),
						MatchJSONPath("$.operation", operation),
						MatchJSONPath("$.state", "COMPLETE"),
						MatchJSONPath("$.errors", BeEmpty()),
					)))
				})
			})

			When("the org deletion times out", func() {
				BeforeEach(func() {
					orgRepo.GetOrgReturns(repositories.OrgRecord{
						GUID:      "cf-org-guid",
						DeletedAt: (time.Now().Add(-180 * time.Second)).Format(time.RFC3339Nano),
					}, nil)
				})

				It("returns a failed status", func() {
					Expect(rr).To(HaveHTTPBody(SatisfyAll(
						MatchJSONPath("$.guid", jobGUID),
						MatchJSONPath("$.links.self.href", defaultServerURL+"/v3/jobs/"+jobGUID),
						MatchJSONPath("$.operation", operation),
						MatchJSONPath("$.state", "FAILED"),
						MatchJSONPath("$.errors", ConsistOf(map[string]interface{}{
							"code":   float64(10008),
							"detail": fmt.Sprintf("CFOrg deletion timed out. Check for lingering resources in the %q namespace", resourceGUID),
							"title":  "CF-UnprocessableEntity",
						})),
					)))
				})
			})

			When("the user does not have permission to see the org", func() {
				BeforeEach(func() {
					orgRepo.GetOrgReturns(repositories.OrgRecord{}, apierrors.NewForbiddenError(nil, repositories.OrgResourceType))
				})

				It("returns a complete status", func() {
					Expect(rr).To(HaveHTTPBody(SatisfyAll(
						MatchJSONPath("$.guid", jobGUID),
						MatchJSONPath("$.links.self.href", defaultServerURL+"/v3/jobs/"+jobGUID),
						MatchJSONPath("$.operation", operation),
						MatchJSONPath("$.state", "COMPLETE"),
						MatchJSONPath("$.errors", BeEmpty()),
					)))
				})
			})

			When("the org has not been marked for deletion", func() {
				BeforeEach(func() {
					orgRepo.GetOrgReturns(repositories.OrgRecord{
						GUID: resourceGUID,
					}, nil)
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
		})
	})
})
