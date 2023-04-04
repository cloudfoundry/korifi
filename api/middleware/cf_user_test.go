package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/middleware"
	"code.cloudfoundry.org/korifi/api/middleware/fake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/utils/clock/testing"
)

var _ = Describe("CfUserMiddleware", func() {
	var (
		cfUserMiddleware    func(http.Handler) http.Handler
		identityProvider    *fake.IdentityProvider
		nsPermissionChecker *fake.NamespacePermissionChecker
		teapotHandler       http.Handler
		cfUserCache         *cache.Expiring
		authInfo            authorization.Info
		ctx                 context.Context
		identity            authorization.Identity
	)

	BeforeEach(func() {
		authInfo = authorization.Info{Token: "a-token"}
		ctx = authorization.NewContext(context.Background(), &authInfo)

		nsPermissionChecker = new(fake.NamespacePermissionChecker)
		nsPermissionChecker.AuthorizedInReturns(true, nil)

		teapotHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})

		identity = authorization.Identity{
			Name: "bob",
			Kind: rbacv1.UserKind,
		}

		identityProvider = new(fake.IdentityProvider)
		identityProvider.GetIdentityReturns(identity, nil)

		cfUserCache = cache.NewExpiringWithClock(testing.NewFakeClock(time.Now()))
		cfUserMiddleware = middleware.CFUser(nsPermissionChecker, identityProvider, "cfroot", cfUserCache)
	})

	JustBeforeEach(func() {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/v3/apps", nil)
		Expect(err).NotTo(HaveOccurred())

		cfUserMiddleware(teapotHandler).ServeHTTP(rr, request)
	})

	It("delegates to the next middleware", func() {
		Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
	})

	It("checks the user identity", func() {
		Expect(identityProvider.GetIdentityCallCount()).To(Equal(1))
		_, actualAuthInfo := identityProvider.GetIdentityArgsForCall(0)
		Expect(actualAuthInfo).To(Equal(authInfo))
	})

	When("checking the identity fails", func() {
		BeforeEach(func() {
			identityProvider.GetIdentityReturns(authorization.Identity{}, errors.New("id-error"))
		})

		It("delegates to the next middleware", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})

		It("doesn't add a warning", func() {
			Expect(rr).NotTo(HaveHTTPHeaderWithValue("X-Cf-Warnings", ContainSubstring("has no CF roles assigned")))
		})
	})

	When("there is no authInfo in the context", func() {
		BeforeEach(func() {
			ctx = context.Background()
		})

		It("delegates to the next middleware", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})

		It("doesn't add a warning", func() {
			Expect(rr).NotTo(HaveHTTPHeaderWithValue("X-Cf-Warnings", ContainSubstring("has no CF roles assigned")))
		})
	})

	It("checks if the user has permissions to the root namespace", func() {
		Expect(nsPermissionChecker.AuthorizedInCallCount()).To(Equal(1))
		c, i, ns := nsPermissionChecker.AuthorizedInArgsForCall(0)
		Expect(c).To(Equal(ctx))
		Expect(i).To(Equal(identity))
		Expect(ns).To(Equal("cfroot"))
	})

	When("checking the user's namespace permissions fails", func() {
		BeforeEach(func() {
			nsPermissionChecker.AuthorizedInReturns(false, errors.New("list-err"))
		})

		It("delegates to the next middleware", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})

		It("doesn't add a warning", func() {
			Expect(rr).NotTo(HaveHTTPHeaderWithValue("X-Cf-Warnings", ContainSubstring("has no CF roles assigned")))
		})
	})

	When("the user is not authorized in the root namespace", func() {
		BeforeEach(func() {
			nsPermissionChecker.AuthorizedInReturns(false, nil)
		})

		It("delegates to the next middleware", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})

		It("sets the X-Cf-Warning header", func() {
			Expect(rr).To(HaveHTTPHeaderWithValue("X-Cf-Warnings", ContainSubstring("has no CF roles assigned")))
		})
	})

	When("on subsequent call with another identity", func() {
		JustBeforeEach(func() {
			rr = httptest.NewRecorder()

			request, err := http.NewRequestWithContext(
				authorization.NewContext(context.Background(), &authorization.Info{
					Token: "b-token",
				}),
				http.MethodGet,
				"http://localhost/bar",
				nil,
			)
			Expect(err).NotTo(HaveOccurred())

			identityProvider.GetIdentityReturns(authorization.Identity{
				Name: "alice",
				Kind: rbacv1.UserKind,
			}, nil)

			cfUserMiddleware(teapotHandler).ServeHTTP(rr, request)
		})

		It("checks if the user has permissions to the root namespace (does not cache previous result)", func() {
			Expect(nsPermissionChecker.AuthorizedInCallCount()).To(Equal(2))
		})
	})

	When("on subsequent calls with the same authInfo", func() {
		JustBeforeEach(func() {
			rr = httptest.NewRecorder()

			request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/bar", nil)
			Expect(err).NotTo(HaveOccurred())

			cfUserMiddleware(teapotHandler).ServeHTTP(rr, request)
		})

		It("delegates to the next middleware", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})

		It("does not check the user's permissions in the root namespace (caches the previous result)", func() {
			Expect(nsPermissionChecker.AuthorizedInCallCount()).To(Equal(1))
		})
	})
})
