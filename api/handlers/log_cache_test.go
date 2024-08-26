package handlers_test

import (
	"encoding/base64"
	"errors"
	"net/http"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"

	. "code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("LogCache", func() {
	var (
		appRepo          *fake.CFAppRepository
		buildRepo        *fake.CFBuildRepository
		logRepo          *fake.LogRepository
		req              *http.Request
		requestValidator *fake.RequestValidator
	)

	BeforeEach(func() {
		requestValidator = new(fake.RequestValidator)
		appRepo = new(fake.CFAppRepository)
		buildRepo = new(fake.CFBuildRepository)
		logRepo = new(fake.LogRepository)

		appRepo.GetAppReturns(repositories.AppRecord{
			GUID:      "app-guid",
			SpaceGUID: "app-space-guid",
		}, nil)

		buildRepo.GetLatestBuildByAppGUIDReturns(repositories.BuildRecord{
			GUID: "build-guid",
		}, nil)

		apiHandler := NewLogCache(
			requestValidator,
			appRepo,
			buildRepo,
			logRepo,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("GET /api/v1/info", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequest("GET", "/api/v1/info", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected info", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(MatchJSON(`{"version":"2.11.4+cf-k8s","vm_uptime":"0"}`)))
		})
	})

	Describe("GET /api/v1/read/<app-guid>", func() {
		var payload *payloads.LogRead

		BeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/api/v1/read/app-guid", nil)
			Expect(err).NotTo(HaveOccurred())

			payload = &payloads.LogRead{
				StartTime:  tools.PtrTo[int64](12345),
				Limit:      tools.PtrTo[int64](1000),
				Descending: true,
			}

			requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(payload)

			logRepo.GetAppLogsReturns([]repositories.LogRecord{
				{Timestamp: 0, Message: "log0"},
				{Timestamp: 1, Message: "log1"},
				{Timestamp: 2, Message: "log2"},
			}, nil)
		})

		It("validates the payload", func() {
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			_, actualPayload := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(actualPayload).To(Equal(payload))
		})

		When("the payload is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesReturns(apierrors.NewUnprocessableEntityError(nil, "invalid-payload"))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("invalid-payload")
			})
		})

		It("gets the app", func() {
			Expect(appRepo.GetAppCallCount()).To(Equal(1))
			_, _, actualAppGUID := appRepo.GetAppArgsForCall(0)
			Expect(actualAppGUID).To(Equal("app-guid"))
		})

		When("the app is not accessible", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("App")
			})
		})

		When("there is an error fetching the app", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		It("gets the latest build for the specified app", func() {
			Expect(buildRepo.GetLatestBuildByAppGUIDCallCount()).To(Equal(1))
			_, _, actualSpaceGUID, actualAppGUID := buildRepo.GetLatestBuildByAppGUIDArgsForCall(0)
			Expect(actualSpaceGUID).To(Equal("app-space-guid"))
			Expect(actualAppGUID).To(Equal("app-guid"))
		})

		When("the build is not accessible", func() {
			BeforeEach(func() {
				buildRepo.GetLatestBuildByAppGUIDReturns(repositories.BuildRecord{}, apierrors.NewForbiddenError(nil, repositories.BuildResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Build")
			})
		})

		When("there is an error fetching the build", func() {
			BeforeEach(func() {
				buildRepo.GetLatestBuildByAppGUIDReturns(repositories.BuildRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		It("gets the build logs", func() {
			Expect(logRepo.GetAppLogsCallCount()).To(Equal(1))
			_, actualAuthInfo, actualMessage := logRepo.GetAppLogsArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualMessage).To(MatchAllFields(Fields{
				"App": MatchFields(IgnoreExtras, Fields{
					"GUID": Equal("app-guid"),
				}),
				"Build": MatchFields(IgnoreExtras, Fields{
					"GUID": Equal("build-guid"),
				}),
				"StartTime":  PointTo(BeEquivalentTo(12345)),
				"Limit":      PointTo(BeEquivalentTo(1000)),
				"Descending": BeTrue(),
			}))
		})

		When("there is an error fetching the logs", func() {
			BeforeEach(func() {
				logRepo.GetAppLogsReturns(nil, errors.New("get-logs-error"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		It("returns the app logs", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.envelopes.batch[0].log.payload", Equal(base64.StdEncoding.EncodeToString([]byte("log0")))),
				MatchJSONPath("$.envelopes.batch[1].log.payload", Equal(base64.StdEncoding.EncodeToString([]byte("log1")))),
				MatchJSONPath("$.envelopes.batch[2].log.payload", Equal(base64.StdEncoding.EncodeToString([]byte("log2")))),
			)))
		})
	})
})
