package apis_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/presenter"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestRootAPI(t *testing.T) {
	spec.Run(t, "GET / endpoint", testRootAPI, spec.Report(report.Terminal{}))
}

func testRootAPI(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	var rr *httptest.ResponseRecorder

	it.Before(func() {
		req, err := http.NewRequest("GET", "/", nil)
		g.Expect(err).NotTo(HaveOccurred())

		rr = httptest.NewRecorder()
		router := mux.NewRouter()

		apiHandler := apis.NewRootHandler(
			logf.Log.WithName("TestRootHandler"),
			defaultServerURL,
		)
		apiHandler.RegisterRoutes(router)

		router.ServeHTTP(rr, req)
	})

	it("returns status 200 OK", func() {
		g.Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
	})

	it("returns Content-Type as JSON in header", func() {
		contentTypeHeader := rr.Header().Get("Content-Type")
		g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
	})

	it("has a non-empty body", func() {
		g.Expect(rr.Body.Bytes()).NotTo(BeEmpty())
	})

	it("matches the expected response body format", func() {
		var resp presenter.RootResponse
		g.Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())

		g.Expect(resp).To(gstruct.MatchAllFields(gstruct.Fields{
			"Links": Equal(map[string]*presenter.APILink{
				"self": {
					Link: presenter.Link{HREF: defaultServerURL},
				},
				"bits_service":        nil,
				"cloud_controller_v2": nil,
				"cloud_controller_v3": {
					Link: presenter.Link{HREF: defaultServerURL + "/v3"},
					Meta: presenter.APILinkMeta{Version: "3.90.0"},
				},
				"network_policy_v0": nil,
				"network_policy_v1": nil,
				"login":             nil,
				"uaa":               nil,
				"credhub":           nil,
				"routing":           nil,
				"logging":           nil,
				"log_cache":         nil,
				"log_stream":        nil,
				"app_ssh":           nil,
			}),
			"CFOnK8s": Equal(true),
		}))
	})
}
