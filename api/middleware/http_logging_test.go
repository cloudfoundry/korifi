package middleware_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"code.cloudfoundry.org/korifi/api/middleware"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HttpLoggingMiddleware", func() {
	var logLines []string

	It("logs the request", func() {
		res := httptest.NewRecorder()
		ctx := logr.NewContext(context.Background(), funcr.NewJSON(func(obj string) {
			logLines = append(logLines, obj)
		}, funcr.Options{}))
		req, err := http.NewRequestWithContext(ctx, "POST", "/path", strings.NewReader("request-body"))
		req.RemoteAddr = "remote-addr"
		Expect(err).NotTo(HaveOccurred())

		middleware.HTTPLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal("POST"))
			Expect(r.URL.Path).To(Equal("/path"))

			w.WriteHeader(http.StatusTeapot)
			fmt.Fprint(w, "hello, world!")
		})).ServeHTTP(res, req)

		Expect(res.Result().StatusCode).To(Equal(http.StatusTeapot))

		Expect(logLines).To(HaveLen(2))

		reqLog := map[string]interface{}{}
		Expect(json.Unmarshal([]byte(logLines[0]), &reqLog)).To(Succeed())
		Expect(reqLog).To(HaveKeyWithValue("msg", "request"))
		Expect(reqLog).To(HaveKeyWithValue("url", "/path"))
		Expect(reqLog).To(HaveKeyWithValue("method", "POST"))
		Expect(reqLog).To(HaveKeyWithValue("remoteAddr", "remote-addr"))
		Expect(reqLog).To(HaveKeyWithValue("contentLength", float64(12)))

		resLog := map[string]interface{}{}
		Expect(json.Unmarshal([]byte(logLines[1]), &resLog)).To(Succeed())
		Expect(resLog).To(HaveKeyWithValue("msg", "response"))
		Expect(resLog).To(HaveKeyWithValue("status", float64(http.StatusTeapot)))
		Expect(resLog).To(HaveKeyWithValue("size", float64(13)))
	})
})
