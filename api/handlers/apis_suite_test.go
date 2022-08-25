package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/correlation"
	"github.com/go-http-utils/headers"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	defaultServerURL = "https://api.example.org"
	jsonHeader       = "application/json"
)

var (
	rr            *httptest.ResponseRecorder
	router        *mux.Router
	serverURL     *url.URL
	ctx           context.Context
	authInfo      authorization.Info
	correlationID string
)

func TestApis(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Apis Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter)))
})

var _ = BeforeEach(func() {
	authInfo = authorization.Info{Token: "a-token"}
	correlationID = generateGUID("corrID")
	ctx = correlation.ContextWithId(authorization.NewContext(context.Background(), &authInfo), correlationID)
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

func expectNotAuthorizedError() {
	expectJSONResponse(http.StatusForbidden, `{
			"errors": [
				{
					"code": 10003,
					"title": "CF-NotAuthorized",
					"detail": "You are not authorized to perform the requested action"
				}
			]
		}`)
}

func expectNotFoundError(detail string) {
	ExpectWithOffset(1, rr).To(HaveHTTPStatus(http.StatusNotFound))
	ExpectWithOffset(1, rr).To(HaveHTTPHeaderWithValue(headers.ContentType, jsonHeader))
	var bodyJSON map[string]interface{}
	Expect(json.Unmarshal(rr.Body.Bytes(), &bodyJSON)).To(Succeed())
	Expect(bodyJSON).To(HaveKey("errors"))
	Expect(bodyJSON["errors"]).To(HaveLen(1))
	Expect(bodyJSON["errors"]).To(ConsistOf(
		gstruct.MatchAllKeys(gstruct.Keys{
			"code":   BeEquivalentTo(10010),
			"title":  Equal("CF-ResourceNotFound"),
			"detail": HavePrefix(detail),
		}),
	))
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

func expectBlobstoreUnavailableError() {
	expectJSONResponse(http.StatusBadGateway, `{
        "errors": [
            {
                "title": "CF-BlobstoreUnavailable",
                "detail": "Error uploading source package to the container registry",
                "code": 150006
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

func expectNotAuthenticatedError() {
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

func expectInvalidAuthError() {
	expectJSONResponse(http.StatusUnauthorized, `{
      "errors": [
          {
            "detail": "Invalid Auth Token",
            "title": "CF-InvalidAuthToken",
            "code": 1000
          }
        ]
    }`)
}

func generateGUID(prefix string) string {
	guid := uuid.NewString()

	return fmt.Sprintf("%s-%s", prefix, guid[:13])
}
