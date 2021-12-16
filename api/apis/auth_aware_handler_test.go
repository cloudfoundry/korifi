package apis_test

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("AuthAwareHandlerFuncWrapper", func() {
	var (
		delegate    *fake.AuthAwareHandlerFunc
		wrappedFunc http.HandlerFunc
	)

	BeforeEach(func() {
		delegate = new(fake.AuthAwareHandlerFunc)
		delegate.Stub = func(_ authorization.Info, w http.ResponseWriter, _ *http.Request) {
			w.Header().Add(headers.ContentType, "application/json")
			w.WriteHeader(http.StatusTeapot)
		}
		wrapper := apis.NewAuthAwareHandlerFuncWrapper(logf.Log.WithName("test"))
		wrappedFunc = wrapper.Wrap(delegate.Spy)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, "GET", "/foo", nil)
		Expect(err).NotTo(HaveOccurred())
		wrappedFunc.ServeHTTP(rr, req)
	})

	It("passes the authorization.Info from the context to the auth aware delegate", func() {
		Expect(delegate.CallCount()).To(Equal(1))
		actualAuthInfo, _, actualReq := delegate.ArgsForCall(0)
		Expect(actualAuthInfo).To(Equal(authInfo))
		Expect(actualReq.URL.Path).To(Equal("/foo"))
	})

	It("returns whatever the delegate returns", func() {
		Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
	})

	When("the authorization.Info object is not available in the context", func() {
		BeforeEach(func() {
			ctx = context.Background()
		})

		It("returns an unknown error", func() {
			expectUnknownError()
		})

		It("does not call the delegate", func() {
			Expect(delegate.CallCount()).To(BeZero())
		})
	})
})
