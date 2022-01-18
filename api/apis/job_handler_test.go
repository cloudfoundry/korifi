package apis_test

import (
	"fmt"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("JobHandler", func() {
	Describe("GET /v3/jobs endpoint", func() {
		const (
			spaceGUID = "my-space-guid"
			appGUID   = "my-app-guid"
			orgGUID   = "my-org-guid"
		)

		var jobGUID string

		JustBeforeEach(func() {
			jobsHandler := apis.NewJobHandler(
				logf.Log.WithName("TestRootHandler"),
				*serverURL,
			)
			jobsHandler.RegisterRoutes(router)

			req, err := http.NewRequest("GET", "/v3/jobs/"+jobGUID, nil)
			Expect(err).NotTo(HaveOccurred())
			router.ServeHTTP(rr, req)
		})

		When("getting an existing job", func() {
			BeforeEach(func() {
				jobGUID = "sync-space.apply_manifest-" + spaceGUID
			})
			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			When("the existing job operation is sync-space.apply-manifest", func() {
				BeforeEach(func() {
					jobGUID = "sync-space.apply_manifest-" + spaceGUID
				})
				It("returns the job", func() {
					Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
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
				}`, defaultServerURL, jobGUID, spaceGUID)), "Response body matches response:")
				})
			})

			When("the existing job operation is app.delete", func() {
				BeforeEach(func() {
					jobGUID = "app.delete-" + appGUID
				})
				It("returns the job", func() {
					Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
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
				}`, defaultServerURL, jobGUID)), "Response body matches response:")
				})
			})

			When("the existing job operation is space.delete", func() {
				BeforeEach(func() {
					jobGUID = "space.delete-" + spaceGUID
				})
				It("returns the job", func() {
					Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
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
				}`, defaultServerURL, jobGUID)), "Response body matches response:")
				})
			})

			When("the existing job operation is space.delete", func() {
				BeforeEach(func() {
					jobGUID = "org.delete-" + orgGUID
				})
				It("returns the job", func() {
					Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
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
				}`, defaultServerURL, jobGUID)), "Response body matches response:")
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
