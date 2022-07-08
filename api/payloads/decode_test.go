package payloads_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Decode", func() {
	var (
		payloadObject DecodeTestPayload
		decodeInput   map[string][]string
		decodeErr     error
	)

	BeforeEach(func() {
		payloadObject = DecodeTestPayload{}
		decodeInput = map[string][]string{
			"key": {"value"},
		}
	})

	JustBeforeEach(func() {
		decodeErr = payloads.Decode(&payloadObject, decodeInput)
	})

	It("decodes into the payload object", func() {
		Expect(decodeErr).NotTo(HaveOccurred())
		Expect(payloadObject.Key).To(Equal("value"))
	})

	When("the input is invalid", func() {
		BeforeEach(func() {
			decodeInput = map[string][]string{
				"key": nil,
			}
		})

		It("returns an error", func() {
			Expect(decodeErr).To(MatchError(ContainSubstring("unable to decode request query parameters")))
		})
	})

	When("the payload input contains unsupported key", func() {
		BeforeEach(func() {
			decodeInput = map[string][]string{
				"foo": {"bar"},
			}
		})

		It("returns an unsupported key error", func() {
			Expect(decodeErr).To(HaveOccurred())
			unknownKeyErr, ok := decodeErr.(apierrors.UnknownKeyError)
			Expect(ok).To(BeTrue())
			Expect(unknownKeyErr.Detail()).To(ContainSubstring("Valid parameters are: 'key'"))
			Expect(unknownKeyErr.Title()).To(Equal("CF-BadQueryParameter"))
			Expect(unknownKeyErr.Code()).To(Equal(10005))
			Expect(unknownKeyErr.HttpStatus()).To(Equal(http.StatusBadRequest))
		})
	})
})

type DecodeTestPayload struct {
	Key string `schema:"key,required"`
}

func (p *DecodeTestPayload) SupportedKeys() []string {
	return []string{"key"}
}
