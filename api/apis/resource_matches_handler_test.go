package apis_test

import (
	"net/http"
	"strings"

	. "code.cloudfoundry.org/korifi/api/apis"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ResourceMatchesHandler", func() {
	var req *http.Request

	BeforeEach(func() {
		handler := NewResourceMatchesHandler(logf.Log.WithName("TestResourceMatchesHandler"))
		handler.RegisterRoutes(router)
	})

	JustBeforeEach(func() {
		router.ServeHTTP(rr, req)
	})

	Describe("Get Resource Match Endpoint", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/resource_matches", strings.NewReader("{}"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns status 201 Created", func() {
			Expect(rr.Code).To(Equal(http.StatusCreated), "Matching HTTP response code:")
		})

		It("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		It("returns a CF API formatted Error response", func() {
			Expect(rr.Body.String()).To(MatchJSON(`{
				"resources": []
			  }`), "Response body matches response:")
		})
	})
})
