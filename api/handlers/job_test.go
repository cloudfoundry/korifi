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

		When("getting an existing job", func() {
			BeforeEach(func() {
				jobGUID = "space.apply_manifest~" + spaceGUID
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK))
			})

			It("returns Content-Type as JSON in header", func() {
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			})

			When("the existing job operation is space.apply-manifest", func() {
				It("returns the job", func() {
					Expect(rr.Body.Bytes()).To(MatchJSONPath("$.guid", jobGUID))
					Expect(rr.Body.Bytes()).To(MatchJSONPath("$.links.space.href", defaultServerURL+"/v3/spaces/"+spaceGUID))
					Expect(rr.Body.Bytes()).To(MatchJSONPath("$.operation", "space.apply_manifest"))
				})
			})

			Describe("job guid validation", func() {
				When("the job guid provided does not have the expected delimiter", func() {
					BeforeEach(func() {
						jobGUID = "job.operation;some-resource-guid"
					})

					It("returns an error", func() {
						expectNotFoundError("Job not found")
					})
				})

				When("the resource identifier portion has a prefixed guid", func() {
					BeforeEach(func() {
						jobGUID = "space.delete~cf-space-a4cd478b-0b02-452f-8498-ce87ec5c6649"
					})

					It("returns status 200 OK", func() {
						Expect(rr.Code).To(Equal(http.StatusOK))
					})
				})
			})

			When("the resource identifier portion does not include a guid", func() {
				BeforeEach(func() {
					jobGUID = "space.apply_manifest~cf-space-staging-space"
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK))
				})
			})
		})

		DescribeTable("delete jobs", func(operation, guid string) {
			jobGUID := operation + "~" + guid
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/jobs/"+jobGUID, nil)
			Expect(err).NotTo(HaveOccurred())

			rr = httptest.NewRecorder()
			routerBuilder.Build().ServeHTTP(rr, req)

			Expect(rr.Body.Bytes()).To(MatchJSONPath("$.guid", jobGUID))
			Expect(rr.Body.Bytes()).To(MatchJSONPath("$.links.self.href", defaultServerURL+"/v3/jobs/"+jobGUID))
			Expect(rr.Body.Bytes()).To(MatchJSONPath("$.operation", operation))
		},

			Entry("app delete", "app.delete", "cf-app-guid"),
			Entry("org delete", "org.delete", "cf-org-guid"),
			Entry("space delete", "space.delete", "cf-space-guid"),
			Entry("route delete", "route.delete", "cf-route-guid"),
			Entry("domain delete", "domain.delete", "cf-domain-guid"),
			Entry("role delete", "role.delete", "cf-role-guid"),
		)
	})
})
