package handlers_test

import (
	"net/http"
	"net/http/httptest"

	"code.cloudfoundry.org/korifi/api/handlers"
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
		)

		BeforeEach(func() {
			spaceGUID = uuid.NewString()
			apiHandler := handlers.NewJob(*serverURL)
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
			Entry("org delete", "org.delete", "cf-org-guid"),
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
	})
})
