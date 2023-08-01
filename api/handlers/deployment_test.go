package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Deployment", func() {
	var (
		requestValidator *fake.RequestValidator
		req              *http.Request
		deploymentsRepo  *fake.CFDeploymentRepository
		runnerInfoRepo   *fake.RunnerInfoRepository
		runnerName       string
	)

	BeforeEach(func() {
		requestValidator = new(fake.RequestValidator)
		deploymentsRepo = new(fake.CFDeploymentRepository)
		runnerInfoRepo = new(fake.RunnerInfoRepository)
		runnerName = "statefulset-runner"

		apiHandler := handlers.NewDeployment(*serverURL, requestValidator, deploymentsRepo, runnerInfoRepo, runnerName)
		routerBuilder.LoadRoutes(apiHandler)

		runnerInfoRepo.GetRunnerInfoReturns(repositories.RunnerInfoRecord{
			Name:       "statefulset-runner",
			Namespace:  "korifi",
			RunnerName: "statefulset-runner",
			Capabilities: v1alpha1.RunnerInfoCapabilities{
				RollingDeploy: true,
			},
		}, nil)
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

			req = createHttpRequest("POST", "/v3/deployments", strings.NewReader("the-payload"))
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.DeploymentCreate{
				Droplet: payloads.DropletGUID{
					Guid: dropletGUID,
				},
				Relationships: &payloads.DeploymentRelationships{
					App: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: appGUID,
						},
					},
				},
			})
		})

		When("runner is not capable of rolling deploy", func() {
			BeforeEach(func() {
				runnerInfoRepo.GetRunnerInfoReturns(repositories.RunnerInfoRecord{
					Name:       "statefulset-runner",
					Namespace:  "korifi",
					RunnerName: "statefulset-runner",
					Capabilities: v1alpha1.RunnerInfoCapabilities{
						RollingDeploy: false,
					},
				}, nil)
			})

			It("returns an error if the RunnerInfo indicates unable to rolling deploy", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusBadRequest))
				Expect(rr).To(HaveHTTPBody("{\"errors\":[{\"detail\":\"The configured runner 'statefulset-runner' does not support rolling deploys\",\"title\":\"CF-RollingDeployNotSupported\",\"code\":42000}]}\n"))
			})
		})

		When("runner is not capable of rolling deploy", func() {
			BeforeEach(func() {
				runnerInfoRepo.GetRunnerInfoReturns(repositories.RunnerInfoRecord{},
					apierrors.NewNotFoundError(errors.New("not found"), "RunnerInfo"))
			})

			It("returns an error if the RunnerInfo indicates unable to rolling deploy", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusBadRequest))
				Expect(rr).To(HaveHTTPBody("{\"errors\":[{\"detail\":\"The configured runner 'statefulset-runner' does not support rolling deploys\",\"title\":\"CF-RollingDeployNotSupported\",\"code\":42000}]}\n"))
			})
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
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-payload"))

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
				requestValidator.DecodeAndValidateJSONPayloadReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
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
