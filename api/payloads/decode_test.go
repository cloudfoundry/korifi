package payloads_test

import (
	"net/http"
	"net/url"
	"strconv"

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
			"key": {"3"},
		}
	})

	JustBeforeEach(func() {
		decodeErr = payloads.Decode(&payloadObject, decodeInput)
	})

	It("decodes into the payload object", func() {
		Expect(decodeErr).NotTo(HaveOccurred())
		Expect(payloadObject.Key).To(Equal(3))
	})

	When("the input cannot be converted", func() {
		BeforeEach(func() {
			decodeInput = map[string][]string{
				"key": {"asd"},
			}
		})

		It("returns a message parse error", func() {
			Expect(decodeErr).To(HaveOccurred())
			invalidRequestErr, ok := decodeErr.(apierrors.MessageParseError)
			Expect(ok).To(BeTrue())
			Expect(invalidRequestErr.Detail()).To(ContainSubstring(`invalid request body`))
			Expect(invalidRequestErr.Title()).To(Equal("CF-MessageParseError"))
			Expect(invalidRequestErr.Code()).To(Equal(1001))
			Expect(invalidRequestErr.HttpStatus()).To(Equal(http.StatusBadRequest))
		})
	})

	When("the input is invalid", func() {
		BeforeEach(func() {
			decodeInput = map[string][]string{
				"key": nil,
			}
		})

		It("returns an error", func() {
			Expect(decodeErr).To(HaveOccurred())
			_, ok := decodeErr.(apierrors.MessageParseError)
			Expect(ok).To(BeTrue())
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
	Key int
}

func (p *DecodeTestPayload) SupportedKeys() []string {
	return []string{"key"}
}

func (p *DecodeTestPayload) DecodeFromURLValues(values url.Values) error {
	var err error
	p.Key, err = strconv.Atoi(values.Get("key"))
	return err
}
