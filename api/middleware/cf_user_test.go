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
	controllersfake "code.cloudfoundry.org/korifi/controllers/fake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CfUserMiddleware", func() {
	var (
		cfUserMiddleware                func(http.Handler) http.Handler
		k8sClient                       *controllersfake.Client
		identityProvider                *fake.IdentityProvider
		teapotHandler                   http.Handler
		cfUserCache                     *cache.Expiring
		unauthenticatedEndpointRegistry *fake.UnauthenticatedEndpointRegistry
		authInfo                        authorization.Info
		ctx                             context.Context
	)

	BeforeEach(func() {
		authInfo = authorization.Info{Token: "a-token"}
		ctx = authorization.NewContext(context.Background(), &authInfo)

		k8sClient = new(controllersfake.Client)
		k8sClient.ListStub = func(_ context.Context, objectsList client.ObjectList, _ ...client.ListOption) error {
			rbList, ok := objectsList.(*rbacv1.RoleBindingList)
			Expect(ok).To(BeTrue())
			*rbList = rbacv1.RoleBindingList{
				Items: []rbacv1.RoleBinding{{
					Subjects: []rbacv1.Subject{{
						Kind: rbacv1.UserKind,
						Name: "bob",
					}},
				}},
			}

			return nil
		}

		teapotHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})

		identityProvider = new(fake.IdentityProvider)
		identityProvider.GetIdentityReturns(authorization.Identity{
			Name: "bob",
			Kind: rbacv1.UserKind,
		}, nil)

		unauthenticatedEndpointRegistry = new(fake.UnauthenticatedEndpointRegistry)
		unauthenticatedEndpointRegistry.IsUnauthenticatedEndpointReturns(false)

		cfUserCache = cache.NewExpiringWithClock(testing.NewFakeClock(time.Now()))
		cfUserMiddleware = middleware.CFUser(k8sClient, identityProvider, "cfroot", cfUserCache, unauthenticatedEndpointRegistry)
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

	When("requesting unauthenticated endpoint", func() {
		BeforeEach(func() {
			unauthenticatedEndpointRegistry.IsUnauthenticatedEndpointReturns(true)
		})

		It("does not check the identity", func() {
			Expect(identityProvider.GetIdentityCallCount()).To(BeZero())
		})

		It("delegates to the next middleware", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})
	})

	When("checking the identity fails", func() {
		BeforeEach(func() {
			identityProvider.GetIdentityReturns(authorization.Identity{}, errors.New("id-error"))
		})

		It("delegates to the next middleware", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})
	})

	When("there is no authInfo in the context", func() {
		BeforeEach(func() {
			ctx = context.Background()
		})

		It("delegates to the next middleware", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})
	})

	It("lists the rolebindings in the cf root namespace", func() {
		Expect(k8sClient.ListCallCount()).To(Equal(1))
		_, _, listOpts := k8sClient.ListArgsForCall(0)
		Expect(listOpts).To(ConsistOf(client.InNamespace("cfroot")))
	})

	When("listing rolebindings fails", func() {
		BeforeEach(func() {
			k8sClient.ListReturns(errors.New("list-err"))
		})

		It("delegates to the next middleware", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})
	})

	When("the user has no rolebindings in the root namespace", func() {
		BeforeEach(func() {
			identityProvider.GetIdentityReturns(authorization.Identity{
				Name: "jim",
				Kind: rbacv1.UserKind,
			}, nil)
		})

		It("delegates to the next middleware", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})

		It("sets the X-Cf-Warning header", func() {
			Expect(rr).To(HaveHTTPHeaderWithValue("X-Cf-Warnings", ContainSubstring("has no CF roles assigned")))
		})
	})

	When("the subject kind does not match", func() {
		BeforeEach(func() {
			identityProvider.GetIdentityReturns(authorization.Identity{
				Name: "bob",
				Kind: rbacv1.ServiceAccountKind,
			}, nil)
		})

		It("delegates to the next middleware", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})

		It("sets the X-Cf-Warning header", func() {
			Expect(rr).To(HaveHTTPHeaderWithValue("X-Cf-Warnings", ContainSubstring("has no CF roles assigned")))
		})
	})

	When("there are no rolebindings in the root namespace", func() {
		BeforeEach(func() {
			k8sClient.ListStub = func(_ context.Context, objectsList client.ObjectList, _ ...client.ListOption) error {
				rbList, ok := objectsList.(*rbacv1.RoleBindingList)
				Expect(ok).To(BeTrue())
				*rbList = rbacv1.RoleBindingList{}

				return nil
			}
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

		It("lists the role bindings again (does not cache previous result)", func() {
			Expect(k8sClient.ListCallCount()).To(Equal(2))
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

		It("does not check identity again (caches the previous result)", func() {
			Expect(k8sClient.ListCallCount()).To(Equal(1))
		})
	})
})
