package apis_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

const (
	defaultServerURL = "https://api.example.org"
	jsonHeader       = "application/json"
)

var (
	rr        *httptest.ResponseRecorder
	req       *http.Request
	router    *mux.Router
	serverURL *url.URL
	ctx       context.Context
)

func TestApis(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Apis Suite")
}

var _ = BeforeEach(func() {
	ctx = context.Background()
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

func initializeProcessRecord(processGUID, spaceGUID, appGUID string) *repositories.ProcessRecord {
	return &repositories.ProcessRecord{
		GUID:             processGUID,
		SpaceGUID:        spaceGUID,
		AppGUID:          appGUID,
		Type:             "web",
		Command:          "rackup",
		DesiredInstances: 1,
		MemoryMB:         256,
		DiskQuotaMB:      1024,
		Ports:            []int32{8080},
		HealthCheck: repositories.HealthCheck{
			Type: "port",
			Data: repositories.HealthCheckData{
				HTTPEndpoint:             "",
				InvocationTimeoutSeconds: 0,
				TimeoutSeconds:           0,
			},
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{},
		CreatedAt:   "",
		UpdatedAt:   "",
	}
}
