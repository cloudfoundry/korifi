package routes_test

import (
	"cloudfoundry.org/cf-k8s-api/routes"
	"github.com/gorilla/mux"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"net/http"
	"net/http/httptest"

	"testing"
)


// initializeAPIRoutes initializes a routes.APIRoutes with empty handler functions for each of its endpoints
//		this is to prevent a panic from RegisterRoutes that occurs when a handler for a route is not set
//	NOTE: Add new handlers here when adding new endpoints!
func initializeAPIRoutes() *routes.APIRoutes {
	emptyHandlerFunc := func(w http.ResponseWriter, r *http.Request) {
	}
	return &routes.APIRoutes{
		RootV3Handler: emptyHandlerFunc,
		RootHandler: emptyHandlerFunc,
	}
}

// createMockHandlerFunc returns a mock handler function that sets the funcRan bool to true when the handler is called
func createMockHandlerFunc(funcRan *bool) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`success`))
		*funcRan = true
	}
}

// sendGetURLToRouter sends a GET request to the provided router at requestURL
func sendGetURLToRouter(requestURL string, router *mux.Router) (*httptest.ResponseRecorder, error) {
	// Create a request to pass to our handler. We don't have any query parameters for now, so we'll
	// pass 'nil' as the third parameter.
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, err
	}
	return sendRequestToRouter(req, router), nil
}

// sendRequestToRouter sends a request to the provided router and returns a response recorder
func sendRequestToRouter(req *http.Request, router *mux.Router) *httptest.ResponseRecorder {
	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	// hit the router
	router.ServeHTTP(rr, req)
	return rr
}


func TestRouter(t *testing.T) {
	spec.Run(t, "object", testRootRoute, spec.Report(report.Terminal{}))
	spec.Run(t, "object", testRootV3Route, spec.Report(report.Terminal{}))
}
func testRootRoute(t *testing.T, when spec.G, it spec.S) {
	Expect := NewWithT(t).Expect

	when("the APIRouter RootHandler is initialized and a mock handler is provided", func() {
		var handlerCalled bool
		var requestURL = "/"

		it.Before(func() {
			handlerCalled = false
			router := mux.NewRouter()
			// create API routes
			apiRoutes := initializeAPIRoutes()
			// This mock handler will be registered with the router by the APIRoutes
			apiRoutes.RootHandler = createMockHandlerFunc(&handlerCalled)
			// Call RegisterRoutes to register all the routes in APIRoutes
			apiRoutes.RegisterRoutes(router)
			// Send a GET request to the requestURL
			sendGetURLToRouter(requestURL, router)
		})

		it("calls the provided mock handler function when GET " + requestURL + " is requested", func() {
			// make sure the provided mockHandlerFunction function was called
			Expect(handlerCalled).To(BeTrue(), "Response body matches mockHandlerFunction response:")
		})

	})

	when("the APIRouter RootHandler is initialized and no handler is provided", func() {
		it("panics when RegisterRoutes is called", func() {
			// This will "catch" the panic from RegisterRoutes, r will be nil if there is no panic
			defer func() {
				r := recover()
				Expect(r).ToNot(BeNil(), "The code did not panic")
			}()

			router := mux.NewRouter()
			// create API routes
			apiRoutes := initializeAPIRoutes()
			apiRoutes.RootHandler = nil
			// Call RegisterRoutes to register all the routes in APIRoutes
			apiRoutes.RegisterRoutes(router)
		})
	})
}

func testRootV3Route(t *testing.T, when spec.G, it spec.S) {
	Expect := NewWithT(t).Expect

	when("the APIRouter RootV3Handler is initialized and a mock handler is provided", func() {
		var handlerCalled bool
		var requestURL = "/v3"

		it.Before(func() {
			handlerCalled = false
			router := mux.NewRouter()
			// create API routes
			apiRoutes := initializeAPIRoutes()
			// This mock handler will be registered with the router by the APIRoutes
			apiRoutes.RootV3Handler = createMockHandlerFunc(&handlerCalled)
			// Call RegisterRoutes to register all the routes in APIRoutes
			apiRoutes.RegisterRoutes(router)
			// Send a GET request to the requestURL
			sendGetURLToRouter(requestURL, router)
		})

		it("calls the provided mock handler function when GET " + requestURL + " is requested", func() {
			// make sure the provided mockHandlerFunction function was called
			Expect(handlerCalled).To(BeTrue(), "Response body matches mockHandlerFunction response:")
		})

	})

	when("the APIRouter RootV3Handler is initialized and no handler is provided", func() {

		it("panics when RegisterRoutes is called", func() {
			// This will "catch" the panic from RegisterRoutes, r will be nil if there is no panic
			defer func() {
				r := recover()
				Expect(r).ToNot(BeNil(), "The code did not panic")
			}()

			router := mux.NewRouter()
			// create API routes
			apiRoutes := initializeAPIRoutes()
			apiRoutes.RootV3Handler = nil
			// Call RegisterRoutes to register all the routes in APIRoutes
			apiRoutes.RegisterRoutes(router)
		})
	})
}