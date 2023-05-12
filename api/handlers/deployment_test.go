package handlers_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Deployment", func() {
	var (
		req             *http.Request
		deploymentsRepo *fake.CFDeploymentRepository
	)

	BeforeEach(func() {
		deploymentsRepo = new(fake.CFDeploymentRepository)

		decoderValidator, err := handlers.NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler := handlers.NewDeployment(*serverURL, decoderValidator, deploymentsRepo)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("POST /v3/deployments", func() {
		BeforeEach(func() {
			deploymentsRepo.CreateDeploymentReturns(repositories.DeploymentRecord{
				GUID:        appGUID,
				DropletGUID: dropletGUID,
				Status: repositories.DeploymentStatus{
					Value:  "deployment-status-value",
					Reason: "deployment-status-reason",
				},
			}, nil)

			req = createHttpRequest("POST", "/v3/deployments", strings.NewReader(fmt.Sprintf(`{
				  "droplet": {
					"guid": "%s"
				  },
				  "relationships": {
					"app": {
					  "data": {
						"guid": "%s"
					  }
					}
				  }
				}`, dropletGUID, appGUID)))
		})

		It("returns a HTTP 201 Created response", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", appGUID),
				MatchJSONPath("$.droplet.guid", dropletGUID),
				MatchJSONPath("$.status.value", "deployment-status-value"),
				MatchJSONPath("$.status.reason", "deployment-status-reason"),
			)))
		})

		It("creates the deployment with the repository", func() {
			Expect(deploymentsRepo.CreateDeploymentCallCount()).To(Equal(1))
			_, actualAuthInfo, createMessage := deploymentsRepo.CreateDeploymentArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(createMessage).To(Equal(repositories.CreateDeploymentMessage{
				AppGUID:     appGUID,
				DropletGUID: dropletGUID,
			}))
		})

		When("the request payload is invalid", func() {
			BeforeEach(func() {
				req = createHttpRequest("POST", "/v3/deployments", strings.NewReader("invalid-payload"))
			})

			It("returns a bad request error", func() {
				expectBadRequestError()
			})
		})

		When("creating the deployment is forbidden", func() {
			BeforeEach(func() {
				deploymentsRepo.CreateDeploymentReturns(repositories.DeploymentRecord{}, apierrors.NewForbiddenError(nil, repositories.DeploymentResourceType))
			})

			It("returns a forbidden error", func() {
				expectNotAuthorizedError()
			})
		})

		When("creating the deployment fails", func() {
			BeforeEach(func() {
				deploymentsRepo.CreateDeploymentReturns(repositories.DeploymentRecord{}, errors.New("create-deployment-error"))
			})

			It("returns an unknown error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("GET /v3/deployments/{guid}", func() {
		BeforeEach(func() {
			deploymentsRepo.GetDeploymentReturns(repositories.DeploymentRecord{
				GUID:        appGUID,
				CreatedAt:   time.Now().Format(repositories.TimestampFormat),
				UpdatedAt:   time.Now().Format(repositories.TimestampFormat),
				DropletGUID: dropletGUID,
				Status: repositories.DeploymentStatus{
					Value:  "deployment-status-value",
					Reason: "deployment-status-reason",
				},
			}, nil)
			req = createHttpRequest("GET", "/v3/deployments/"+appGUID, nil)
		})

		It("returns a HTTP 200 OK response", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", appGUID),
				MatchJSONPath("$.droplet.guid", dropletGUID),
				MatchJSONPath("$.status.value", "deployment-status-value"),
				MatchJSONPath("$.status.reason", "deployment-status-reason"),
			)))
		})

		It("gets the deployment from the repository", func() {
			Expect(deploymentsRepo.GetDeploymentCallCount()).To(Equal(1))
			_, actualAuthInfo, deploymentGUID := deploymentsRepo.GetDeploymentArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(deploymentGUID).To(Equal(appGUID))
		})

		When("getting the deployment is forbidden", func() {
			BeforeEach(func() {
				deploymentsRepo.GetDeploymentReturns(repositories.DeploymentRecord{}, apierrors.NewForbiddenError(nil, repositories.DeploymentResourceType))
			})

			It("returns a not found error", func() {
				expectNotFoundError(repositories.DeploymentResourceType)
			})
		})

		When("getting the deployment fails", func() {
			BeforeEach(func() {
				deploymentsRepo.GetDeploymentReturns(repositories.DeploymentRecord{}, errors.New("get-deployment-error"))
			})

			It("returns an unknown error", func() {
				expectUnknownError()
			})
		})
	})
})
