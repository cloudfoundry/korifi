package payloads_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/labels"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var validator validation.DecoderValidator

var _ = BeforeEach(func() {
	var err error
	validator = validation.NewDefaultDecoderValidator()
	Expect(err).NotTo(HaveOccurred())
})

func TestPayloads(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Payloads Suite")
}

func expectUnprocessableEntityError(err error, detail string) {
	GinkgoHelper()

	Expect(err).To(HaveOccurred())
	Expect(err).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
	Expect(err.(apierrors.UnprocessableEntityError).Detail()).To(ContainSubstring(detail))
}

func createJSONRequest(payload any) *http.Request {
	GinkgoHelper()

	body, err := json.Marshal(payload)
	Expect(err).NotTo(HaveOccurred())

	req, err := http.NewRequest("", "", bytes.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	return req
}

func createYAMLRequest(payload any) *http.Request {
	GinkgoHelper()

	body, err := yaml.Marshal(payload)
	Expect(err).NotTo(HaveOccurred())

	req, err := http.NewRequest("", "", bytes.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	return req
}

type keyedPayload[T any] interface {
	validation.KeyedPayload
	*T
}

func decodeQuery[T any, PT keyedPayload[T]](query string) (PT, error) {
	req, err := http.NewRequest("GET", "http://foo.bar/?"+query, nil)
	Expect(err).NotTo(HaveOccurred())

	var actual PT = new(T)
	decodeErr := validator.DecodeAndValidateURLValues(req, actual)

	return actual, decodeErr
}

func parseLabelSelector(s string) labels.Selector {
	selector, err := labels.Parse(s)
	Expect(err).NotTo(HaveOccurred())
	return selector
}
