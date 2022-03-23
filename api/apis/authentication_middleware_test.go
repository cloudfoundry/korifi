package apis_test

import (
	"errors"
	"fmt"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"github.com/go-http-utils/headers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const authHeader = "Authorization: something"

var _ = Describe("Authentication Middleware", func() {
	var (
		authMiddleware   *apis.AuthenticationMiddleware
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

		authInfoParser = new(fake.AuthInfoParser)
		identityProvider = new(fake.IdentityProvider)
		authMiddleware = apis.NewAuthenticationMiddleware(
			log.Log.WithName("AuthenticationMiddlewareTest"),
			authInfoParser,
			identityProvider,
		)

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

			It("does not inject an authorization.Info in the context", func() {
				_, ok := authorization.InfoFromContext(actualReq.Context())
				Expect(ok).To(BeFalse())
			})
		})

		Describe("/v3", func() {
			BeforeEach(func() {
				requestPath = "/v3"
			})

			It("passes through", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
			})

			It("does not inject an authorization.Info in the context", func() {
				_, ok := authorization.InfoFromContext(actualReq.Context())
				Expect(ok).To(BeFalse())
			})
		})

		Describe("/api/v1/info", func() {
			BeforeEach(func() {
				requestPath = "/api/v1/info"
			})

			It("passes through", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
			})

			It("does not inject an authorization.Info in the context", func() {
				_, ok := authorization.InfoFromContext(actualReq.Context())
				Expect(ok).To(BeFalse())
			})
		})

		Describe("/api/v1/read/:guid", func() {
			When("given an arbitrary guid", func() {
				BeforeEach(func() {
					guid := uuid.NewString()
					requestPath = fmt.Sprintf("/api/v1/read/%s", guid)
				})

				It("passes through", func() {
					Expect(rr).To(HaveHTTPStatus(http.StatusTeapot), fmt.Sprintf("Request path: %s", requestPath))
				})

				It("does not inject an authorization.Info in the context", func() {
					_, ok := authorization.InfoFromContext(actualReq.Context())
					Expect(ok).To(BeFalse(), fmt.Sprintf("Request path: %s", requestPath))
				})
			})
		})
	})

	Describe("endpoints requiring authentication", func() {
		BeforeEach(func() {
			requestPath = "/v3/apps"
			authInfoParser.ParseReturns(authorization.Info{Token: "the-token"}, nil)
			identityProvider.GetIdentityReturns(authorization.Identity{}, nil)
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
})
