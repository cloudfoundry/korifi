package handlers_test

import (
	"errors"
	"net/http"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LogCache", func() {
	var (
		appRepo          *fake.CFAppRepository
		buildRepo        *fake.CFBuildRepository
		appLogsReader    *fake.AppLogsReader
		req              *http.Request
		requestValidator *fake.RequestValidator
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		buildRepo = new(fake.CFBuildRepository)
		appLogsReader = new(fake.AppLogsReader)
		requestValidator = new(fake.RequestValidator)

		apiHandler := NewLogCache(
			appRepo,
			buildRepo,
			appLogsReader,
			requestValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("the GET /api/v1/info endpoint", func() {
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

	Describe("the GET /api/v1/read/<app-guid> endpoint", func() {
		var payload *payloads.LogRead

		BeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/api/v1/read/the-app-guid", nil)
			Expect(err).NotTo(HaveOccurred())

			payload = &payloads.LogRead{}
			requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(payload)

			appLogsReader.ReadReturns([]repositories.LogRecord{
				{
					Message: "message-1",
				},
				{
					Message: "message-2",
				},
			}, nil)
		})

		It("lists the log envelopes", func() {
			Expect(appLogsReader.ReadCallCount()).To(Equal(1))
			_, _, actualAuthInfo, appGUID, payload := appLogsReader.ReadArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(appGUID).To(Equal("the-app-guid"))
			Expect(payload).To(BeZero())

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.envelopes.batch[0].log.payload", "bWVzc2FnZS0x"),
				MatchJSONPath("$.envelopes.batch[1].log.payload", "bWVzc2FnZS0y"),
			)))
		})

		It("validates the payload", func() {
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			_, actualPayload := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(actualPayload).To(Equal(payload))
		})

		When("query parameters are specified", func() {
			BeforeEach(func() {
				payload.Descending = true
				payload.EnvelopeTypes = []string{"LOG", "TIMER"}
				payload.Limit = 1000
				payload.StartTime = -6795364578871345152
			})

			It("filters the log records accordingly", func() {
				Expect(appLogsReader.ReadCallCount()).To(Equal(1))
				_, _, _, _, payload := appLogsReader.ReadArgsForCall(0)
				Expect(payload).To(Equal(payloads.LogRead{
					StartTime:     -6795364578871345152,
					EnvelopeTypes: []string{"LOG", "TIMER"},
					Limit:         1000,
					Descending:    true,
				}))
			})
		})

		When("the payload is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesReturns(apierrors.NewUnprocessableEntityError(nil, "boom"))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("boom")
			})
		})

		When("the action returns a not-found error", func() {
			BeforeEach(func() {
				appLogsReader.ReadReturns(nil, apierrors.NewNotFoundError(nil, repositories.AppResourceType))
			})
			It("elevates the error", func() {
				expectNotFoundError("App")
			})
		})

		When("the action returns a random error", func() {
			BeforeEach(func() {
				appLogsReader.ReadReturns(nil, errors.New("i-am-made-up"))
			})
			It("returns an Unknown error", func() {
				expectUnknownError()
			})
		})
	})
})
