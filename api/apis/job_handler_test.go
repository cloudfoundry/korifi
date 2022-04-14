package apis_test

import (
	"fmt"
	"net/http"

	"code.cloudfoundry.org/korifi/api/apis"

	"github.com/go-http-utils/headers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("JobHandler", func() {
	Describe("GET /v3/jobs endpoint", func() {
		var (
			resourceGUID string
			jobGUID      string
			req          *http.Request
		)

		BeforeEach(func() {
			resourceGUID = uuid.NewString()
			jobsHandler := apis.NewJobHandler(
				logf.Log.WithName("TestJobsHandler"),
				*serverURL,
			)
			jobsHandler.RegisterRoutes(router)
		})

		JustBeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/jobs/"+jobGUID, nil)
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		})

		When("getting an existing job", func() {
			BeforeEach(func() {
				jobGUID = "space.apply_manifest-" + resourceGUID
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK))
			})

			It("returns Content-Type as JSON in header", func() {
				Expect(rr).To(HaveHTTPHeaderWithValue(headers.ContentType, jsonHeader))
			})

			When("the existing job operation is space.apply-manifest", func() {
				It("returns the job", func() {
					Expect(rr.Body).To(MatchJSON(fmt.Sprintf(`{
						"created_at": "",
						"errors": null,
						"guid": "%[2]s",
						"links": {
							"self": {
								"href": "%[1]s/v3/jobs/%[2]s"
							},
							"space": {
								"href": "%[1]s/v3/spaces/%[3]s"
							}
						},
						"operation": "space.apply_manifest",
						"state": "COMPLETE",
						"updated_at": "",
						"warnings": null
						}`, defaultServerURL, jobGUID, resourceGUID)))
				})
			})

			When("the existing job operation is app.delete", func() {
				BeforeEach(func() {
					jobGUID = "app.delete-" + resourceGUID
				})

				It("returns the job", func() {
					Expect(rr.Body).To(MatchJSON(fmt.Sprintf(`{
						"created_at": "",
						"errors": null,
						"guid": "%[2]s",
						"links": {
							"self": {
								"href": "%[1]s/v3/jobs/%[2]s"
							}
						},
						"operation": "app.delete",
						"state": "COMPLETE",
						"updated_at": "",
						"warnings": null
					}`, defaultServerURL, jobGUID)))
				})
			})

			When("the existing job operation is org.delete", func() {
				BeforeEach(func() {
					resourceGUID = "cf-org-" + uuid.NewString()
					jobGUID = "org.delete-" + resourceGUID
				})

				It("returns the job", func() {
					Expect(rr.Body).To(MatchJSON(fmt.Sprintf(`{
						"created_at": "",
						"errors": null,
						"guid": "%[2]s",
						"links": {
							"self": {
								"href": "%[1]s/v3/jobs/%[2]s"
							}
						},
						"operation": "org.delete",
						"state": "COMPLETE",
						"updated_at": "",
						"warnings": null
					}`, defaultServerURL, jobGUID)))
				})
			})

			When("the existing job operation is space.delete", func() {
				BeforeEach(func() {
					resourceGUID = "cf-space-" + uuid.NewString()
					jobGUID = "space.delete-" + resourceGUID
				})

				It("returns the job", func() {
					Expect(rr.Body).To(MatchJSON(fmt.Sprintf(`{
						"created_at": "",
						"errors": null,
						"guid": "%[2]s",
						"links": {
							"self": {
								"href": "%[1]s/v3/jobs/%[2]s"
							}
						},
						"operation": "space.delete",
						"state": "COMPLETE",
						"updated_at": "",
						"warnings": null
					}`, defaultServerURL, jobGUID)))
				})
			})

			When("the existing job operation is route.delete", func() {
				BeforeEach(func() {
					resourceGUID = "cf-route-" + uuid.NewString()
					jobGUID = "route.delete-" + resourceGUID
				})

				It("returns the job", func() {
					Expect(rr.Body).To(MatchJSON(fmt.Sprintf(`{
						"created_at": "",
						"errors": null,
						"guid": "%[2]s",
						"links": {
							"self": {
								"href": "%[1]s/v3/jobs/%[2]s"
							}
						},
						"operation": "route.delete",
						"state": "COMPLETE",
						"updated_at": "",
						"warnings": null
					}`, defaultServerURL, jobGUID)))
				})
			})
		})

		When("guid provided is not a valid job guid", func() {
			BeforeEach(func() {
				jobGUID = "some-guid"
			})

			It("return an error", func() {
				expectNotFoundError("Job not found")
			})
		})
	})
})
