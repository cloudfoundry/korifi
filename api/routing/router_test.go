package routing_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/api/routing"
)

func handler(r *http.Request) (*routing.Response, error) {
	return routing.NewResponse(http.StatusTeapot).WithBody("hello"), nil
}

func middleware(key, value string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add(key, value)
			next.ServeHTTP(w, r)
		})
	}
}

type routable struct{}

func (r routable) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: http.MethodGet, Pattern: "/hello/auth", Handler: handler},
	}
}

func (r routable) UnauthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: http.MethodGet, Pattern: "/hello", Handler: handler},
	}
}

var _ = Describe("Router", func() {
	var (
		routerBuilder *routing.RouterBuilder
		router        http.Handler
	)

	BeforeEach(func() {
		routerBuilder = routing.NewRouterBuilder()
	})

	JustBeforeEach(func() {
		routerBuilder.LoadRoutes(routable{})
		router = routerBuilder.Build()
	})

	It("can serve unauthenticated routes", func() {
		res, err := mkReq(router, http.MethodGet, "/hello")
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(HaveHTTPStatus(http.StatusTeapot))
	})

	It("can serve authenticated routes", func() {
		res, err := mkReq(router, http.MethodGet, "/hello/auth")
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(HaveHTTPStatus(http.StatusTeapot))
	})

	When("a common middleware is used", func() {
		BeforeEach(func() {
			routerBuilder.UseMiddleware(
				middleware("X-Test", "foo"),
				middleware("X-Test-Other", "bar"),
			)
		})

		It("applies to both unauthenticated and authenticated endpoints", func() {
			res, err := mkReq(router, http.MethodGet, "/hello")
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(HaveHTTPHeaderWithValue("X-Test", "foo"))
			Expect(res).To(HaveHTTPHeaderWithValue("X-Test-Other", "bar"))

			res, err = mkReq(router, http.MethodGet, "/hello/auth")
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(HaveHTTPHeaderWithValue("X-Test", "foo"))
			Expect(res).To(HaveHTTPHeaderWithValue("X-Test-Other", "bar"))
		})
	})

	When("an auth middleware is used", func() {
		BeforeEach(func() {
			routerBuilder.UseAuthMiddleware(
				middleware("X-Test", "foo"),
				middleware("X-Test-Other", "bar"),
			)
		})

		It("applies to only authenticated endpoints", func() {
			res, err := mkReq(router, http.MethodGet, "/hello")
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(HaveHTTPHeaderWithValue("X-Test", "foo"))
			Expect(res).NotTo(HaveHTTPHeaderWithValue("X-Test-Other", "bar"))

			res, err = mkReq(router, http.MethodGet, "/hello/auth")
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(HaveHTTPHeaderWithValue("X-Test", "foo"))
			Expect(res).To(HaveHTTPHeaderWithValue("X-Test-Other", "bar"))
		})
	})

	When("using both common and auth middleware", func() {
		BeforeEach(func() {
			routerBuilder.UseMiddleware(
				middleware("X-Test", "foo"),
			)
			routerBuilder.UseAuthMiddleware(
				middleware("X-Test", "bar"),
			)
		})

		It("applies auth after common", func() {
			res, err := mkReq(router, http.MethodGet, "/hello/auth")
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(HaveHTTPHeaderWithValue("X-Test", "bar"))
		})
	})
})

func mkReq(handler http.Handler, method, url string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr.Result(), nil
}
