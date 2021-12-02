package apis_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	defaultServerURL = "https://api.example.org"
	jsonHeader       = "application/json"
)

var (
	rr        *httptest.ResponseRecorder
	router    *mux.Router
	serverURL *url.URL
	ctx       context.Context
	authInfo  authorization.Info
)

func TestApis(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Apis Suite")
}

var _ = BeforeEach(func() {
	authInfo = authorization.Info{Token: "a-token"}
	ctx = authorization.NewContext(context.Background(), &authInfo)
	rr = httptest.NewRecorder()
	router = mux.NewRouter()

	var err error
	serverURL, err = url.Parse(defaultServerURL)
	Expect(err).NotTo(HaveOccurred())
})

func defaultServerURI(paths ...string) string {
	return fmt.Sprintf("%s%s", defaultServerURL, strings.Join(paths, ""))
}

func expectJSONResponse(status int, body string) {
	ExpectWithOffset(2, rr).To(HaveHTTPStatus(status))
	ExpectWithOffset(2, rr).To(HaveHTTPHeaderWithValue("Content-Type", jsonHeader))
	ExpectWithOffset(2, rr).To(HaveHTTPBody(MatchJSON(body)))
}

func expectUnknownError() {
	expectJSONResponse(http.StatusInternalServerError, `{
			"errors": [
				{
					"title": "UnknownError",
					"detail": "An unknown error occurred.",
					"code": 10001
				}
			]
		}`)
}

func expectNotFoundError(detail string) {
	expectJSONResponse(http.StatusNotFound, fmt.Sprintf(`{
			"errors": [
				{
					"code": 10010,
					"title": "CF-ResourceNotFound",
					"detail": %q
				}
			]
		}`, detail))
}

func expectUnprocessableEntityError(detail string) {
	expectJSONResponse(http.StatusUnprocessableEntity, fmt.Sprintf(`{
			"errors": [
				{
					"detail": %q,
					"title": "CF-UnprocessableEntity",
					"code": 10008
				}
			]
		}`, detail))
}

func expectBadRequestError() {
	expectJSONResponse(http.StatusBadRequest, `{
        "errors": [
            {
                "title": "CF-MessageParseError",
                "detail": "Request invalid due to parse error: invalid request body",
                "code": 1001
            }
        ]
    }`)
}

func expectUnknownKeyError(detail string) {
	expectJSONResponse(http.StatusBadRequest, fmt.Sprintf(`{
		"errors": [
			{
				"code": 10005,
				"title": "CF-BadQueryParameter",
				"detail": %q
			}
		]
	}`, detail))
}

func expectUnauthorizedError() {
	expectJSONResponse(http.StatusUnauthorized, `{
        "errors": [
            {
                "detail": "Authentication error",
                "title": "CF-NotAuthenticated",
                "code": 10002
            }
        ]
    }`)
}
