package handlers_test

import (
	"encoding/json"
	"net/http"

	apis "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/presenter"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("RootHandler", func() {
	var req *http.Request

	BeforeEach(func() {
		apiHandler := apis.NewRootHandler(
			defaultServerURL,
		)
		router.RegisterHandler("handler", apiHandler)
	})

	JustBeforeEach(func() {
		router.ServeHTTP(rr, req)
	})

	Describe("GET / endpoint", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequest("GET", "/", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns status 200 OK", func() {
			Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		It("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		It("has a non-empty body", func() {
			Expect(rr.Body.Bytes()).NotTo(BeEmpty())
		})

		It("matches the expected response body format", func() {
			var resp presenter.RootResponse
			Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())

			Expect(resp).To(gstruct.MatchAllFields(gstruct.Fields{
				"Links": Equal(map[string]*presenter.APILink{
					"self": {
						Link: presenter.Link{HRef: defaultServerURL},
					},
					"bits_service":        nil,
					"cloud_controller_v2": nil,
					"cloud_controller_v3": {
						Link: presenter.Link{HRef: defaultServerURL + "/v3"},
						Meta: presenter.APILinkMeta{Version: presenter.V3APIVersion},
					},
					"network_policy_v0": nil,
					"network_policy_v1": nil,
					"login": {
						Link: presenter.Link{HRef: defaultServerURL},
					},
					"uaa":     nil,
					"credhub": nil,
					"routing": nil,
					"logging": nil,
					"log_cache": {
						Link: presenter.Link{HRef: defaultServerURL},
					},
					"log_stream": nil,
					"app_ssh":    nil,
				}),
				"CFOnK8s": Equal(true),
			}))
		})
	})
})
