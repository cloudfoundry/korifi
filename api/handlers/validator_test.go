package handlers_test

import (
	"net/http"
	"net/url"
	"strconv"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"github.com/jellydator/validation"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validator", func() {
	Describe("DecodeAndValidateURLValues", func() {
		var (
			requestValidator handlers.DecoderValidator
			decoded          DecodeTestPayload
			decodeErr        error
			requestUrl       string
		)

		BeforeEach(func() {
			requestValidator = handlers.NewDefaultDecoderValidator()
			requestUrl = "http://foo.com?key=3"
			decoded = DecodeTestPayload{}
		})

		JustBeforeEach(func() {
			url, err := url.Parse(requestUrl)
			Expect(err).NotTo(HaveOccurred())
			decodeErr = requestValidator.DecodeAndValidateURLValues(&http.Request{
				URL: url,
			}, &decoded)
		})

		It("decodes into the payload object", func() {
			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(decoded.Key).To(Equal(3))
		})

		When("the input cannot be converted", func() {
			BeforeEach(func() {
				requestUrl = "http://foo.com?key=asd"
			})

			It("returns a message parse error", func() {
				Expect(decodeErr).To(HaveOccurred())
				invalidRequestErr, ok := decodeErr.(apierrors.MessageParseError)
				Expect(ok).To(BeTrue())
				Expect(invalidRequestErr.Detail()).To(ContainSubstring("invalid request body"))
				Expect(invalidRequestErr.Title()).To(Equal("CF-MessageParseError"))
				Expect(invalidRequestErr.Code()).To(Equal(1001))
				Expect(invalidRequestErr.HttpStatus()).To(Equal(http.StatusBadRequest))
			})
		})

		When("the input is invalid", func() {
			BeforeEach(func() {
				requestUrl = "http://foo.com?key=-3"
			})

			It("returns an unprocessable entity error", func() {
				Expect(decodeErr).To(HaveOccurred())
				unprocessableEntityErr, ok := decodeErr.(apierrors.UnprocessableEntityError)
				Expect(ok).To(BeTrue())
				Expect(unprocessableEntityErr.Detail()).To(ContainSubstring("must be no less than 0"))
				Expect(unprocessableEntityErr.Title()).To(Equal("CF-UnprocessableEntity"))
				Expect(unprocessableEntityErr.Code()).To(Equal(10008))
				Expect(unprocessableEntityErr.HttpStatus()).To(Equal(http.StatusUnprocessableEntity))
			})
		})

		When("the payload input contains unsupported key", func() {
			BeforeEach(func() {
				requestUrl = "http://foo.com?foo=bar"
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
})

type DecodeTestPayload struct {
	Key int
}

func (p DecodeTestPayload) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Key, validation.Min(0)),
	)
}

func (p *DecodeTestPayload) SupportedKeys() []string {
	return []string{"key"}
}

func (p *DecodeTestPayload) DecodeFromURLValues(values url.Values) error {
	var err error
	p.Key, err = strconv.Atoi(values.Get("key"))
	return err
}
