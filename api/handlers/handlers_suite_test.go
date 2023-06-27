package handlers_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/routing"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	defaultServerURL = "https://api.example.org"
)

var (
	rr            *httptest.ResponseRecorder
	routerBuilder *routing.RouterBuilder
	serverURL     *url.URL
	ctx           context.Context
	authInfo      authorization.Info
)

func TestHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Handlers Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter)))
})

var _ = BeforeEach(func() {
	authInfo = authorization.Info{Token: "a-token"}
	ctx = authorization.NewContext(context.Background(), &authInfo)
	rr = httptest.NewRecorder()
	routerBuilder = routing.NewRouterBuilder()

	var err error
	serverURL, err = url.Parse(defaultServerURL)
	Expect(err).NotTo(HaveOccurred())
})

func expectErrorResponse(status int, title, detail string, code int) {
	GinkgoHelper()

	Expect(rr).To(HaveHTTPStatus(status))
	Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
	Expect(rr).To(HaveHTTPBody(SatisfyAll(
		MatchJSONPath("$.errors[0].title", MatchRegexp(title)),
		MatchJSONPath("$.errors[0].detail", MatchRegexp(detail)),
		MatchJSONPath("$.errors[0].code", BeEquivalentTo(code)),
	)))
}

func expectUnknownError() {
	GinkgoHelper()

	expectErrorResponse(http.StatusInternalServerError, "UnknownError", "An unknown error occurred.", 10001)
}

func expectNotAuthorizedError() {
	GinkgoHelper()

	expectErrorResponse(http.StatusForbidden, "CF-NotAuthorized", "You are not authorized to perform the requested action", 10003)
}

func expectNotFoundError(resourceType string) {
	GinkgoHelper()

	expectErrorResponse(http.StatusNotFound, "CF-ResourceNotFound", resourceType+" not found. Ensure it exists and you have access to it.", 10010)
}

func expectUnprocessableEntityError(detail string) {
	GinkgoHelper()

	expectErrorResponse(http.StatusUnprocessableEntity, "CF-UnprocessableEntity", detail, 10008)
}

func expectBadRequestError() {
	GinkgoHelper()

	expectErrorResponse(http.StatusBadRequest, "CF-MessageParseError", "Request invalid due to parse error: invalid request body", 1001)
}

func expectBlobstoreUnavailableError() {
	GinkgoHelper()

	expectErrorResponse(http.StatusBadGateway, "CF-BlobstoreUnavailable", "Error uploading source package to the container registry", 150006)
}

func expectUnknownKeyError(detail string) {
	GinkgoHelper()

	expectErrorResponse(http.StatusBadRequest, "CF-BadQueryParameter", detail, 10005)
}

func generateGUID(prefix string) string {
	guid := uuid.NewString()

	return fmt.Sprintf("%s-%s", prefix, guid[:13])
}

func decodeAndValidateJSONPayloadStub[T any](desiredPayload *T) func(_ *http.Request, decodedPayload any) error {
	return func(_ *http.Request, decodedPayload any) error {
		GinkgoHelper()

		decodedPayloadPtr, ok := decodedPayload.(*T)
		Expect(ok).To(BeTrue())

		*decodedPayloadPtr = *desiredPayload

		return nil
	}
}

type keyedPayloadImpl[P any] interface {
	handlers.KeyedPayload
	*P
}

func decodeAndValidateURLValuesStub[P any, I keyedPayloadImpl[P]](desiredPayload I) func(*http.Request, handlers.KeyedPayload) error {
	return func(_ *http.Request, output handlers.KeyedPayload) error {
		GinkgoHelper()

		outputPtr, ok := output.(I)
		Expect(ok).To(BeTrue())

		*outputPtr = *desiredPayload
		return nil
	}
}

func bodyString(r *http.Request) string {
	GinkgoHelper()

	bodyBytes, err := io.ReadAll(r.Body)
	Expect(err).NotTo(HaveOccurred())
	return string(bodyBytes)
}
