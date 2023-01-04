package routing_test

import (
	"errors"
	"net/http"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/routing"
	"code.cloudfoundry.org/korifi/api/routing/fake"
	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Handler", func() {
	var (
		delegate *fake.Handler
		handler  routing.Handler
		response *routing.Response
	)

	BeforeEach(func() {
		response = routing.NewResponse(http.StatusTeapot)
		delegate = new(fake.Handler)
		delegate.Stub = func(*http.Request) (*routing.Response, error) {
			return response, nil
		}
		handler = routing.Handler(delegate.Spy)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequest("GET", "/foo", nil)
		Expect(err).NotTo(HaveOccurred())
		handler.ServeHTTP(rr, req)
	})

	It("passes the http.Request to the delegate", func() {
		Expect(delegate.CallCount()).To(Equal(1))
		actualReq := delegate.ArgsForCall(0)
		Expect(actualReq.URL.Path).To(Equal("/foo"))
	})

	It("returns whatever the delegate returns", func() {
		Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
	})

	It("does not set content type header on the response", func() {
		Expect(rr.Header()).NotTo(HaveKey(headers.ContentType))
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
			delegate.Stub = func(*http.Request) (*routing.Response, error) {
				return nil, errors.New("delegateErr")
			}
		})

		It("returns an unknown error response", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusInternalServerError))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(MatchJSON(`{
				"errors": [
					{
						"title": "UnknownError",
						"detail": "An unknown error occurred.",
						"code": 10001
					}
				]
			}`)))
		})
	})

	When("the delegate returns an API error", func() {
		BeforeEach(func() {
			delegate.Stub = func(*http.Request) (*routing.Response, error) {
				return nil, apierrors.NewUnprocessableEntityError(errors.New("foo"), "bar")
			}
		})

		It("presents the error", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(MatchJSON(`{
				"errors": [
					{
						"title": "CF-UnprocessableEntity",
						"detail": "bar",
						"code": 10008
					}
				]
			}`)))
		})
	})
})
