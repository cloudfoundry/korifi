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
			requestValidator *handlers.DecoderValidator
			decoded          DecodeTestPayload
			urlValues        map[string][]string
			decodeErr        error
		)

		BeforeEach(func() {
			var err error
			requestValidator, err = handlers.NewDefaultDecoderValidator()
			Expect(err).NotTo(HaveOccurred())

			decoded = DecodeTestPayload{}
			urlValues = map[string][]string{
				"key": {"3"},
			}
		})

		JustBeforeEach(func() {
			decodeErr = requestValidator.DecodeAndValidateURLValues(&http.Request{
				Form: urlValues,
			}, &decoded)
		})

		It("decodes into the payload object", func() {
			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(decoded.Key).To(Equal(3))
		})

		When("the input cannot be converted", func() {
			BeforeEach(func() {
				urlValues = map[string][]string{
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
				urlValues = map[string][]string{
					"key": {"-3"},
				}
			})

			It("returns an error", func() {
				Expect(decodeErr).To(MatchError(ContainSubstring("must be no less than 0")))
			})
		})

		When("the payload input contains unsupported key", func() {
			BeforeEach(func() {
				urlValues = map[string][]string{
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
