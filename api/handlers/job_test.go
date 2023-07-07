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
	"code.cloudfoundry.org/korifi/tools"

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
			orgRepo   *fake.CFOrgRepository
			spaceRepo *fake.CFSpaceRepository
		)

		BeforeEach(func() {
			spaceGUID = uuid.NewString()

			orgRepo = new(fake.CFOrgRepository)
			spaceRepo = new(fake.CFSpaceRepository)
			apiHandler := handlers.NewJob(*serverURL, orgRepo, spaceRepo, 0)
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
					orgRepo.GetOrgUnfilteredReturns(repositories.OrgRecord{
						GUID:      "cf-org-guid",
						DeletedAt: tools.PtrTo(time.Now()),
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
					orgRepo.GetOrgUnfilteredReturns(repositories.OrgRecord{}, apierrors.NewNotFoundError(nil, repositories.OrgResourceType))
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
					orgRepo.GetOrgUnfilteredReturns(repositories.OrgRecord{
						GUID:      "cf-org-guid",
						DeletedAt: tools.PtrTo(time.Now().Add(-180 * time.Second)),
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
							"detail": fmt.Sprintf("Org deletion timed out. Check for remaining resources in the %q namespace", resourceGUID),
							"title":  "CF-UnprocessableEntity",
						})),
					)))
				})
			})

			When("the user does not have permission to see the org", func() {
				BeforeEach(func() {
					orgRepo.GetOrgUnfilteredReturns(repositories.OrgRecord{}, apierrors.NewForbiddenError(nil, repositories.OrgResourceType))
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
					orgRepo.GetOrgUnfilteredReturns(repositories.OrgRecord{
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

		Describe("space delete", func() {
			const (
				operation    = "space.delete"
				resourceGUID = "cf-space-guid"
			)

			BeforeEach(func() {
				jobGUID = operation + "~" + resourceGUID
			})

			When("the space deletion is in progress", func() {
				BeforeEach(func() {
					spaceRepo.GetSpaceReturns(repositories.SpaceRecord{
						GUID:      "cf-space-guid",
						DeletedAt: tools.PtrTo(time.Now()),
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

			When("the space does not exist", func() {
				BeforeEach(func() {
					spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, apierrors.NewNotFoundError(nil, repositories.SpaceResourceType))
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

			When("the space deletion times out", func() {
				BeforeEach(func() {
					spaceRepo.GetSpaceReturns(repositories.SpaceRecord{
						GUID:      "cf-space-guid",
						DeletedAt: tools.PtrTo(time.Now().Add(-180 * time.Second)),
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
							"detail": fmt.Sprintf("Space deletion timed out. Check for remaining resources in the %q namespace", resourceGUID),
							"title":  "CF-UnprocessableEntity",
						})),
					)))
				})
			})

			When("the user does not have permission to see the space", func() {
				BeforeEach(func() {
					spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, apierrors.NewForbiddenError(nil, repositories.SpaceResourceType))
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

			When("the space has not been marked for deletion", func() {
				BeforeEach(func() {
					spaceRepo.GetSpaceReturns(repositories.SpaceRecord{
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
