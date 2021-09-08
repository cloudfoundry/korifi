package apis_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/presenters"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type FakeRouteRepo struct {
	FetchRouteFunc func(_ client.Client, _ string) (repositories.RouteRecord, error)
}

func (f *FakeRouteRepo) ConfigureClient(_ *rest.Config) (client.Client, error) {
	err := networkingv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	fakeClientBuilder := &fake.ClientBuilder{}
	return fakeClientBuilder.WithScheme(scheme.Scheme).WithObjects(&networkingv1alpha1.CFRoute{}).Build(), nil
}

func (f *FakeRouteRepo) FetchRoute(client client.Client, routeGUID string) (repositories.RouteRecord, error) {
	return f.FetchRouteFunc(client, routeGUID)
}

type FakeDomainRepo struct {
	FetchDomainFunc func(_ client.Client, _ string) (repositories.DomainRecord, error)
}

func (f *FakeDomainRepo) ConfigureClient(_ *rest.Config) (client.Client, error) {
	err := networkingv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	fakeClientBuilder := &fake.ClientBuilder{}
	return fakeClientBuilder.WithScheme(scheme.Scheme).WithObjects(&networkingv1alpha1.CFRoute{}).Build(), nil
}

func (f *FakeDomainRepo) FetchDomain(client client.Client, domainGUID string) (repositories.DomainRecord, error) {
	return f.FetchDomainFunc(client, domainGUID)
}

var (
	fetchRouteResponse  repositories.RouteRecord
	fetchRouteErr       error
	fetchDomainResponse repositories.DomainRecord
	fetchDomainErr      error
)

func TestRoute(t *testing.T) {
	spec.Run(t, "RouteHandler", testRouteHandler, spec.Report(report.Terminal{}))
}

func testRouteHandler(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	var (
		rr *httptest.ResponseRecorder
	)

	when("the GET /v3/routes/:guid  endpoint returns successfully", func() {
		it.Before(func() {
			fetchRouteResponse = repositories.RouteRecord{
				GUID:      "test-route-guid",
				SpaceGUID: "test-space-guid",
				DomainRef: repositories.DomainRecord{
					GUID: "test-domain-guid",
				},
				Host:     "test-route-name",
				Protocol: "http",
			}
			fetchRouteErr = nil

			fetchDomainResponse = repositories.DomainRecord{
				Name: "example.org",
				GUID: "test-domain-guid",
			}
			fetchDomainErr = nil

			req, err := http.NewRequest("GET", "/v3/routes/test-route-guid", nil)
			g.Expect(err).NotTo(HaveOccurred())

			rr = httptest.NewRecorder()
			routeHandler := apis.RouteHandler{
				ServerURL: defaultServerURL,
				RouteRepo: &FakeRouteRepo{
					FetchRouteFunc: func(_ client.Client, _ string) (repositories.RouteRecord, error) {
						return fetchRouteResponse, fetchRouteErr
					},
				},
				DomainRepo: &FakeDomainRepo{
					FetchDomainFunc: func(_ client.Client, _ string) (repositories.DomainRecord, error) {
						return fetchDomainResponse, fetchDomainErr
					},
				},
				Logger:    logf.Log.WithName("TestRouteHandler"),
				K8sConfig: &rest.Config{}, // required for k8s client (transitive dependency from route repo)
			}

			handler := http.HandlerFunc(routeHandler.RouteGetHandler)

			handler.ServeHTTP(rr, req)
		})

		it("returns status 200 OK", func() {
			httpStatus := rr.Code
			g.Expect(httpStatus).Should(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("returns the Route in the response", func() {
			expectedBody, err := json.Marshal(presenters.RouteResponse{
				GUID:     "test-route-guid",
				Protocol: "http",
				Host:     "test-route-name",
				URL:      "test-route-name.example.org",
				Relationships: presenters.Relationships{
					"space": presenters.Relationship{
						GUID: "test-space-guid",
					},
					"domain": presenters.Relationship{
						GUID: "test-domain-guid",
					},
				},
				Metadata: presenters.Metadata{
					Labels:      map[string]string{},
					Annotations: map[string]string{},
				},
				Links: presenters.RouteLinks{
					Self: presenters.Link{
						HREF: defaultServerURI("/v3/routes/test-route-guid"),
					},
					Space: presenters.Link{
						HREF: defaultServerURI("/v3/spaces/test-space-guid"),
					},
					Domain: presenters.Link{
						HREF: defaultServerURI("/v3/domains/test-domain-guid"),
					},
					Destinations: presenters.Link{
						HREF: defaultServerURI("/v3/routes/test-route-guid/destinations"),
					},
				},
			})

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
		})

		// Test returns a 404
		when("the route cannot be found", func() {
			it.Before(func() {
				fetchRouteResponse = repositories.RouteRecord{}
				fetchRouteErr = repositories.NotFoundError{Err: errors.New("not found")}

				req, err := http.NewRequest("GET", "/v3/routes/my-route-guid", nil)
				g.Expect(err).NotTo(HaveOccurred())

				rr = httptest.NewRecorder()
				apiHandler := apis.RouteHandler{
					ServerURL: defaultServerURL,
					RouteRepo: &FakeRouteRepo{
						FetchRouteFunc: func(_ client.Client, _ string) (repositories.RouteRecord, error) {
							return fetchRouteResponse, fetchRouteErr
						},
					},
					Logger:    logf.Log.WithName("TestRouteHandler"),
					K8sConfig: &rest.Config{},
				}

				handler := http.HandlerFunc(apiHandler.RouteGetHandler)

				handler.ServeHTTP(rr, req)
			})

			it("returns a CF API formatted Error response", func() {
				expectedBody, err := json.Marshal(presenters.ErrorsResponse{Errors: []presenters.PresentedError{{
					Title:  "Route not found",
					Detail: "CF-ResourceNotFound",
					Code:   10010,
				}}})

				httpStatus := rr.Code
				g.Expect(httpStatus).Should(Equal(http.StatusNotFound), "Matching HTTP response code:")

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
			})
		})

		when("the route's domain cannot be found", func() {
			it.Before(func() {
				fetchRouteResponse = repositories.RouteRecord{
					GUID:      "test-route-guid",
					SpaceGUID: "test-space-guid",
					DomainRef: repositories.DomainRecord{
						GUID: "test-domain-guid",
					},
					Host:     "test-route-name",
					Protocol: "http",
				}
				fetchRouteErr = nil

				fetchDomainResponse = repositories.DomainRecord{}
				fetchDomainErr = repositories.NotFoundError{Err: errors.New("not found")}

				req, err := http.NewRequest("GET", "/v3/routes/my-route-guid", nil)
				g.Expect(err).NotTo(HaveOccurred())

				rr = httptest.NewRecorder()
				apiHandler := apis.RouteHandler{
					ServerURL: defaultServerURL,
					RouteRepo: &FakeRouteRepo{
						FetchRouteFunc: func(_ client.Client, _ string) (repositories.RouteRecord, error) {
							return fetchRouteResponse, fetchRouteErr
						},
					},
					DomainRepo: &FakeDomainRepo{
						FetchDomainFunc: func(_ client.Client, _ string) (repositories.DomainRecord, error) {
							return fetchDomainResponse, fetchDomainErr
						},
					},
					Logger:    logf.Log.WithName("TestRouteHandler"),
					K8sConfig: &rest.Config{},
				}

				handler := http.HandlerFunc(apiHandler.RouteGetHandler)

				handler.ServeHTTP(rr, req)
			})

			it("returns a CF API formatted Error response", func() {
				expectedBody, err := json.Marshal(presenters.ErrorsResponse{Errors: []presenters.PresentedError{{
					Title:  "UnknownError",
					Detail: "An unknown error occurred.",
					Code:   10001,
				}}})

				httpStatus := rr.Code
				g.Expect(httpStatus).Should(Equal(http.StatusInternalServerError), "Matching HTTP response code:")

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
			})
		})

		when("there is some other error fetching the route", func() {
			it.Before(func() {
				fetchRouteResponse = repositories.RouteRecord{}
				fetchRouteErr = errors.New("unknown!")

				req, err := http.NewRequest("GET", "/v3/routes/my-route-guid", nil)
				g.Expect(err).NotTo(HaveOccurred())

				rr = httptest.NewRecorder()
				apiHandler := apis.RouteHandler{
					ServerURL: defaultServerURL,
					RouteRepo: &FakeRouteRepo{
						FetchRouteFunc: func(_ client.Client, _ string) (repositories.RouteRecord, error) {
							return fetchRouteResponse, fetchRouteErr
						},
					},
					Logger:    logf.Log.WithName("TestRouteHandler"),
					K8sConfig: &rest.Config{},
				}

				handler := http.HandlerFunc(apiHandler.RouteGetHandler)

				handler.ServeHTTP(rr, req)
			})

			it("returns a CF API formatted Error response", func() {
				expectedBody, err := json.Marshal(presenters.ErrorsResponse{Errors: []presenters.PresentedError{{
					Title:  "UnknownError",
					Detail: "An unknown error occurred.",
					Code:   10001,
				}}})

				httpStatus := rr.Code
				g.Expect(httpStatus).Should(Equal(http.StatusInternalServerError), "Matching HTTP response code:")

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
			})
		})
	})
}
