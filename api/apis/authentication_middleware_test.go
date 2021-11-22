package apis_test

import (
	"errors"
	"net/http"
	"net/http/httptest"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/provider/fake"
	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const authHeader = "Authorization: something"

var _ = Describe("Authentication Middleware", func() {
	var (
		authMiddleware   *apis.AuthenticationMiddleware
		nextHandler      http.Handler
		rr               *httptest.ResponseRecorder
		identityProvider *fake.IdentityProvider
		requestPath      string
	)

	BeforeEach(func() {
		rr = httptest.NewRecorder()
		nextHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })

		identityProvider = new(fake.IdentityProvider)
		authMiddleware = apis.NewAuthenticationMiddleware(identityProvider)

		requestPath = ""
	})

	JustBeforeEach(func() {
		request, err := http.NewRequest(http.MethodGet, "http://localhost"+requestPath, nil)
		Expect(err).NotTo(HaveOccurred())
		request.Header.Add(headers.Authorization, authHeader)
		authMiddleware.Middleware(nextHandler).ServeHTTP(rr, request)
	})

	Describe("endpoints not requiring authentication", func() {
		Describe("/", func() {
			BeforeEach(func() {
				requestPath = "/"
			})

			It("passes through", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
			})
		})

		Describe("/v3", func() {
			BeforeEach(func() {
				requestPath = "/v3"
			})

			It("passes through", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
			})
		})
	})

	Describe("endpoints requiring authentication", func() {
		BeforeEach(func() {
			requestPath = "/v3/apps"
			identityProvider.GetIdentityReturns(authorization.Identity{}, nil)
		})

		It("verifies authentication and passes through", func() {
			Expect(identityProvider.GetIdentityCallCount()).To(Equal(1))
			_, actualAuthHeader := identityProvider.GetIdentityArgsForCall(0)
			Expect(actualAuthHeader).To(Equal(authHeader))
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})

		When("authentication is not provided", func() {
			BeforeEach(func() {
				identityProvider.GetIdentityReturns(authorization.Identity{}, authorization.NotAuthenticatedError{})
			})

			It("returns a CF-NotAuthenticated error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusUnauthorized))
				Expect(rr).To(HaveHTTPBody(MatchJSON(`{
                    "errors": [
                        {
                            "detail": "Authentication error",
                            "title": "CF-NotAuthenticated",
                            "code": 10002
                        }
                    ]
                }`)))
			})
		})

		When("authentication is not valid", func() {
			BeforeEach(func() {
				identityProvider.GetIdentityReturns(authorization.Identity{}, authorization.InvalidAuthError{})
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

		When("an unexpected authentication error occurs", func() {
			BeforeEach(func() {
				identityProvider.GetIdentityReturns(authorization.Identity{}, errors.New("boo"))
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
	})
})
