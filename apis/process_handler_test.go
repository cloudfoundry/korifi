package apis_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	. "code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("ProcessHandler", func() {
	const (
		processGUID = "test-process-guid"
	)

	var (
		rr            *httptest.ResponseRecorder
		req           *http.Request
		processRepo   *fake.CFProcessRepository
		clientBuilder *fake.ClientBuilder
		router        *mux.Router
	)

	getRR := func() *httptest.ResponseRecorder { return rr }

	BeforeEach(func() {
		processRepo = new(fake.CFProcessRepository)
		clientBuilder = new(fake.ClientBuilder)

		rr = httptest.NewRecorder()
		router = mux.NewRouter()
		serverURL, err := url.Parse(defaultServerURL)
		Expect(err).NotTo(HaveOccurred())
		apiHandler := NewProcessHandler(
			logf.Log.WithName(testAppHandlerLoggerName),
			*serverURL,
			processRepo,
			clientBuilder.Spy,
			&rest.Config{},
		)
		apiHandler.RegisterRoutes(router)
	})

	JustBeforeEach(func() {
		router.ServeHTTP(rr, req)
	})

	Describe("the GET /v3/processes/:guid/sidecars endpoint", func() {
		BeforeEach(func() {
			processRepo.FetchProcessReturns(repositories.ProcessRecord{}, nil)

			var err error
			req, err = http.NewRequest("GET", "/v3/processes/"+processGUID+"/sidecars", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns a canned response with the processGUID in it", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
					"pagination": {
						"total_results": 0,
						"total_pages": 1,
						"first": {
							"href": "%[1]s/v3/processes/%[2]s/sidecars?page=1"
						},
						"last": {
							"href": "%[1]s/v3/processes/%[2]s/sidecars?page=1"
						},
						"next": null,
						"previous": null
					},
					"resources": []
				}`, defaultServerURL, processGUID)), "Response body matches response:")
			})
		})

		When("on the sad path and", func() {
			When("the process doesn't exist", func() {
				BeforeEach(func() {
					processRepo.FetchProcessReturns(repositories.ProcessRecord{}, repositories.NotFoundError{})
				})

				itRespondsWithNotFound("Process not found", getRR)
			})

			When("there is some other error fetching the process", func() {
				BeforeEach(func() {
					processRepo.FetchProcessReturns(repositories.ProcessRecord{}, errors.New("unknown!"))
				})

				itRespondsWithUnknownError(getRR)
			})
		})
	})

})
