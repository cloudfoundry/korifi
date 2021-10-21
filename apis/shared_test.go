package apis_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"code.cloudfoundry.org/cf-k8s-api/repositories"

	. "github.com/onsi/gomega"
)

const (
	defaultServerURL = "https://api.example.org"
	jsonHeader       = "application/json"
)

func defaultServerURI(paths ...string) string {
	return fmt.Sprintf("%s%s", defaultServerURL, strings.Join(paths, ""))
}

func expectJSONResponse(rr *httptest.ResponseRecorder, status int, body string) {
	Expect(rr).To(HaveHTTPStatus(status))
	Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", jsonHeader))
	Expect(rr).To(HaveHTTPBody(MatchJSON(body)))
}

func expectUnknownError(rr *httptest.ResponseRecorder) {
	expectJSONResponse(rr, http.StatusInternalServerError, `{
			"errors": [
				{
					"title": "UnknownError",
					"detail": "An unknown error occurred.",
					"code": 10001
				}
			]
		}`)
}

func expectNotFoundError(rr *httptest.ResponseRecorder, detail string) {
	expectJSONResponse(rr, http.StatusNotFound, fmt.Sprintf(`{
			"errors": [
				{
					"code": 10010,
					"title": "CF-ResourceNotFound",
					"detail": %q
				}
			]
		}`, detail))
}

func expectUnprocessableEntityError(rr *httptest.ResponseRecorder, detail string) {
	expectJSONResponse(rr, http.StatusUnprocessableEntity, fmt.Sprintf(`{
			"errors": [
				{
					"detail": %q,
					"title": "CF-UnprocessableEntity",
					"code": 10008
				}
			]
		}`, detail))
}

func initializeProcessRecord(processGUID, spaceGUID, appGUID string) *repositories.ProcessRecord {
	return &repositories.ProcessRecord{
		GUID:        processGUID,
		SpaceGUID:   spaceGUID,
		AppGUID:     appGUID,
		Type:        "web",
		Command:     "rackup",
		Instances:   1,
		MemoryMB:    256,
		DiskQuotaMB: 1024,
		Ports:       []int32{8080},
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
