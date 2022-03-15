package apis_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("AuthAwareHandlerFuncWrapper", func() {
	var (
		delegate    *fake.AuthAwareHandlerFunc
		wrappedFunc http.HandlerFunc
		response    *apis.HandlerResponse
	)

	BeforeEach(func() {
		response = apis.NewHandlerResponse(http.StatusTeapot)
		delegate = new(fake.AuthAwareHandlerFunc)
		delegate.Stub = func(_ authorization.Info, _ *http.Request) (*apis.HandlerResponse, error) {
			return response, nil
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
		actualAuthInfo, actualReq := delegate.ArgsForCall(0)
		Expect(actualAuthInfo).To(Equal(authInfo))
		Expect(actualReq.URL.Path).To(Equal("/foo"))
	})

	It("returns whatever the delegate returns", func() {
		Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
	})

	It("does not set content type header on the response", func() {
		Expect(rr.Header()).NotTo(HaveKey(headers.ContentType))
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

	When("the response body is not nil", func() {
		BeforeEach(func() {
			response = response.WithBody("some-response-content")
		})

		It("sets the application/json content type in the response", func() {
			Expect(rr).To(HaveHTTPHeaderWithValue(headers.ContentType, "application/json"))
		})

		It("sets the body into the response", func() {
			Expect(rr).To(HaveHTTPBody(ContainSubstring("some-response-content")))
		})
	})

	When("the response sets header values", func() {
		BeforeEach(func() {
			response = response.WithHeader(headers.Location, "/home")
			response = response.WithHeader(headers.Link, "link")
		})

		It("sets the header on the response", func() {
			Expect(rr).To(HaveHTTPHeaderWithValue(headers.Location, "/home"))
			Expect(rr).To(HaveHTTPHeaderWithValue(headers.Link, "link"))
		})
	})

	When("the delegate returns an unknown error", func() {
		BeforeEach(func() {
			delegate.Stub = func(_ authorization.Info, _ *http.Request) (*apis.HandlerResponse, error) {
				return nil, errors.New("delegateErr")
			}
		})

		It("returns an unknown error response", func() {
			expectUnknownError()
		})
	})

	When("the delegate returns an unprocessable entity error", func() {
		BeforeEach(func() {
			delegate.Stub = func(_ authorization.Info, _ *http.Request) (*apis.HandlerResponse, error) {
				return nil, apierrors.NewUnprocessableEntityError(errors.New("foo"), "bar")
			}
		})

		It("presents the error", func() {
			expectUnprocessableEntityError("bar")
		})
	})

	When("the delegate returns a not authenticated error", func() {
		BeforeEach(func() {
			delegate.Stub = func(_ authorization.Info, _ *http.Request) (*apis.HandlerResponse, error) {
				return nil, apierrors.NewNotAuthenticatedError(errors.New("foo"))
			}
		})

		It("presents the error", func() {
			expectNotAuthenticatedError()
		})
	})

	When("the delegate returns an invalid auth error", func() {
		BeforeEach(func() {
			delegate.Stub = func(_ authorization.Info, _ *http.Request) (*apis.HandlerResponse, error) {
				return nil, apierrors.NewInvalidAuthError(errors.New("foo"))
			}
		})

		It("presents the error", func() {
			expectInvalidAuthError()
		})
	})

	When("the delegate returns a wrapped api error", func() {
		BeforeEach(func() {
			delegate.Stub = func(_ authorization.Info, _ *http.Request) (*apis.HandlerResponse, error) {
				return nil, fmt.Errorf("wrapping %w", apierrors.NewUnprocessableEntityError(errors.New("foo"), "bar"))
			}
		})

		It("presents the error", func() {
			expectUnprocessableEntityError("bar")
		})
	})
})
