package handlers_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("AuthAwareHandlerFuncWrapper", func() {
	var (
		delegate    *fake.AuthAwareHandlerFunc
		wrappedFunc http.Handler
		response    *handlers.HandlerResponse
	)

	BeforeEach(func() {
		response = handlers.NewHandlerResponse(http.StatusTeapot)
		delegate = new(fake.AuthAwareHandlerFunc)
		delegate.Stub = func(_ context.Context, _ logr.Logger, _ authorization.Info, _ *http.Request) (*handlers.HandlerResponse, error) {
			return response, nil
		}
		wrappedFunc = handlers.NewAuthenticatedWrapper(logf.Log.WithName("test"), delegate.Spy)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, "GET", "/foo", nil)
		Expect(err).NotTo(HaveOccurred())
		wrappedFunc.ServeHTTP(rr, req)
	})

	It("passes the authorization.Info from the context to the auth aware delegate", func() {
		Expect(delegate.CallCount()).To(Equal(1))
		_, _, actualAuthInfo, actualReq := delegate.ArgsForCall(0)
		Expect(actualAuthInfo).To(Equal(authInfo))
		Expect(actualReq.URL.Path).To(Equal("/foo"))
	})

	Describe("logging the correlationID", func() {
		var buf *bytes.Buffer

		BeforeEach(func() {
			buf = new(bytes.Buffer)
			GinkgoWriter.TeeTo(buf)
		})

		AfterEach(func() {
			GinkgoWriter.ClearTeeWriters()
		})

		It("passes a logger with correlation id set to the delegate", func() {
			Expect(delegate.CallCount()).To(Equal(1))
			_, logger, _, _ := delegate.ArgsForCall(0)
			logger.Info("bar")
			Expect(buf.String()).To(ContainSubstring(correlationID))
		})
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
			delegate.Stub = func(_ context.Context, _ logr.Logger, _ authorization.Info, _ *http.Request) (*handlers.HandlerResponse, error) {
				return nil, errors.New("delegateErr")
			}
		})

		It("returns an unknown error response", func() {
			expectUnknownError()
		})
	})

	When("the delegate returns an unprocessable entity error", func() {
		BeforeEach(func() {
			delegate.Stub = func(_ context.Context, _ logr.Logger, _ authorization.Info, _ *http.Request) (*handlers.HandlerResponse, error) {
				return nil, apierrors.NewUnprocessableEntityError(errors.New("foo"), "bar")
			}
		})

		It("presents the error", func() {
			expectUnprocessableEntityError("bar")
		})
	})

	When("the delegate returns a not authenticated error", func() {
		BeforeEach(func() {
			delegate.Stub = func(_ context.Context, _ logr.Logger, _ authorization.Info, _ *http.Request) (*handlers.HandlerResponse, error) {
				return nil, apierrors.NewNotAuthenticatedError(errors.New("foo"))
			}
		})

		It("presents the error", func() {
			expectNotAuthenticatedError()
		})
	})

	When("the delegate returns an invalid auth error", func() {
		BeforeEach(func() {
			delegate.Stub = func(_ context.Context, _ logr.Logger, _ authorization.Info, _ *http.Request) (*handlers.HandlerResponse, error) {
				return nil, apierrors.NewInvalidAuthError(errors.New("foo"))
			}
		})

		It("presents the error", func() {
			expectInvalidAuthError()
		})
	})

	When("the delegate returns a wrapped api error", func() {
		BeforeEach(func() {
			delegate.Stub = func(_ context.Context, _ logr.Logger, _ authorization.Info, _ *http.Request) (*handlers.HandlerResponse, error) {
				return nil, fmt.Errorf("wrapping %w", apierrors.NewUnprocessableEntityError(errors.New("foo"), "bar"))
			}
		})

		It("presents the error", func() {
			expectUnprocessableEntityError("bar")
		})
	})

	When("using the unauthenticated wrapper", func() {
		BeforeEach(func() {
			ctx = context.Background()
			wrappedFunc = handlers.NewUnauthenticatedWrapper(logf.Log.WithName("test"), delegate.Spy)
		})

		It("passes empty auth.Info to the auth aware delegate", func() {
			Expect(delegate.CallCount()).To(Equal(1))
			_, _, actualAuthInfo, actualReq := delegate.ArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authorization.Info{}))
			Expect(actualReq.URL.Path).To(Equal("/foo"))
		})

		It("returns whatever the delegate returns", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})
	})
})
