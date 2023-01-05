package middleware_test

import (
	"errors"
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/middleware"
	"code.cloudfoundry.org/korifi/api/middleware/fake"

	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const authHeader = "Authorization: something"

var _ = Describe("Authentication Middleware", func() {
	var (
		authMiddleware   func(http.Handler) http.Handler
		nextHandler      http.Handler
		identityProvider *fake.IdentityProvider
		authInfoParser   *fake.AuthInfoParser
		requestPath      string
		actualReq        *http.Request
	)

	BeforeEach(func() {
		nextHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actualReq = r
			w.WriteHeader(http.StatusTeapot)
		})

		requestPath = "/v3/apps"

		authInfoParser = new(fake.AuthInfoParser)
		authInfoParser.ParseReturns(authorization.Info{Token: "the-token"}, nil)

		identityProvider = new(fake.IdentityProvider)
		identityProvider.GetIdentityReturns(authorization.Identity{}, nil)

		authMiddleware = middleware.Authentication(
			authInfoParser,
			identityProvider,
		)
	})

	JustBeforeEach(func() {
		request, err := http.NewRequest(http.MethodGet, "http://localhost"+requestPath, nil)
		Expect(err).NotTo(HaveOccurred())
		request.Header.Add(headers.Authorization, authHeader)
		authMiddleware(nextHandler).ServeHTTP(rr, request)
	})

	It("verifies authentication and passes through", func() {
		Expect(authInfoParser.ParseCallCount()).To(Equal(1))
		Expect(authInfoParser.ParseArgsForCall(0)).To(Equal(authHeader))

		Expect(identityProvider.GetIdentityCallCount()).To(Equal(1))
		_, actualAuthInfo := identityProvider.GetIdentityArgsForCall(0)
		Expect(actualAuthInfo).To(Equal(authorization.Info{Token: "the-token"}))

		Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
	})

	It("parses the Authorization header into an authorization.Info and injects it in the request context", func() {
		actualAuthInfo, ok := authorization.InfoFromContext(actualReq.Context())
		Expect(ok).To(BeTrue())
		Expect(actualAuthInfo).To(Equal(authorization.Info{Token: "the-token"}))
	})

	When("parsing the Authorization header fails", func() {
		BeforeEach(func() {
			authInfoParser.ParseReturns(authorization.Info{}, apierrors.NewInvalidAuthError(nil))
		})

		It("returns a CF-InvalidAuthToken error", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusUnauthorized))
			Expect(rr).To(HaveHTTPBody(MatchJSON(`{
                    "errors": [
                    {
                        "detail": "Invalid Auth Token",
                        "title": "CF-InvalidAuthToken",
                        "code": 1000
                    }
                    ]
                }`)))
		})
	})

	When("Authorization header parsing fails for unknown reason", func() {
		BeforeEach(func() {
			authInfoParser.ParseReturns(authorization.Info{}, errors.New("what happened?"))
		})

		It("returns a CF-Unknown error", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusInternalServerError))
			Expect(rr).To(HaveHTTPBody(MatchJSON(`{
                    "errors": [
                    {
                        "detail": "An unknown error occurred.",
                        "title": "UnknownError",
                        "code": 10001
                    }
                    ]
                }`)))
		})
	})

	When("getting the identity fails", func() {
		BeforeEach(func() {
			identityProvider.GetIdentityReturns(authorization.Identity{}, apierrors.NewInvalidAuthError(nil))
		})

		It("returns a CF-InvalidAuthToken error", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusUnauthorized))
			Expect(rr).To(HaveHTTPBody(MatchJSON(`{
                    "errors": [
                    {
                        "detail": "Invalid Auth Token",
                        "title": "CF-InvalidAuthToken",
                        "code": 1000
                    }
                    ]
                }`)))
		})
	})
})
