package handlers_test

import (
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

type example struct {
	Foo int    `json:"foo"`
	Bar string `json:"bar"`
}

var _ = Describe("Decoder", func() {
	var (
		r         *http.Request
		ctype     string
		body      string
		onlyKnown bool
		res       *example
		resErr    error
	)

	BeforeEach(func() {
		ctype = ""
		body = ""
		onlyKnown = false
	})

	JustBeforeEach(func() {
		var err error
		r, err = http.NewRequest("foo", "bar", strings.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		r.Header.Set("Content-Type", ctype)

		res, resErr = handlers.BodyToObject[example](r, onlyKnown)
	})

	Context("JSON", func() {
		BeforeEach(func() {
			ctype = "application/json"
			body = `{"foo": 42, "bar": "meaning"}`
		})

		It("can load a request body into an object", func() {
			Expect(resErr).NotTo(HaveOccurred())
			Expect(res).To(PointTo(Equal(example{Foo: 42, Bar: "meaning"})))
		})
	})

	Context("YAML", func() {
		BeforeEach(func() {
			ctype = "application/x-yaml"
			body = `
foo: 42
bar: meaning`
		})

		It("can load a request body into an object", func() {
			Expect(resErr).NotTo(HaveOccurred())
			Expect(res).To(PointTo(Equal(example{Foo: 42, Bar: "meaning"})))
		})
	})

	When("not JSON or YAML", func() {
		BeforeEach(func() {
			ctype = "something/else"
		})

		It("fails", func() {
			Expect(resErr).To(MatchError(ContainSubstring("unsupported Content-Type")))
		})
	})

	When("unknown fields passed", func() {
		Context("JSON", func() {
			BeforeEach(func() {
				ctype = "application/json"
				body = `{"jim": "bowen"}`
			})

			When("other fields allowed", func() {
				BeforeEach(func() {
					onlyKnown = false
				})

				It("succeeds", func() {
					Expect(resErr).ToNot(HaveOccurred())
				})
			})

			When("other fields not allowed", func() {
				BeforeEach(func() {
					onlyKnown = true
				})

				It("fails", func() {
					Expect(resErr).To(MatchError(ContainSubstring("unknown field")))
					Expect(resErr).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
				})
			})
		})

		Context("YAML", func() {
			BeforeEach(func() {
				ctype = "text/x-yaml"
				body = `jim: bowen`
			})

			When("other fields allowed", func() {
				BeforeEach(func() {
					onlyKnown = false
				})

				It("succeeds", func() {
					Expect(resErr).ToNot(HaveOccurred())
				})
			})

			When("other fields not allowed", func() {
				BeforeEach(func() {
					onlyKnown = true
				})

				It("fails", func() {
					Expect(resErr).To(MatchError(ContainSubstring("not found")))
					Expect(resErr).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
				})
			})
		})
	})

	When("body has invalid syntax", func() {
		Context("JSON", func() {
			BeforeEach(func() {
				ctype = "application/json"
				body = "{"
			})

			It("fails", func() {
				Expect(resErr).To(BeAssignableToTypeOf(apierrors.MessageParseError{}))
			})
		})

		Context("YAML", func() {
			BeforeEach(func() {
				ctype = "text/yaml"
				body = "{"
			})

			It("fails", func() {
				Expect(resErr).To(BeAssignableToTypeOf(apierrors.MessageParseError{}))
			})
		})
	})

	When("a field has the wrong type", func() {
		Context("JSON", func() {
			BeforeEach(func() {
				ctype = "application/json"
				body = `{"foo": "bar"}`
			})

			It("fails", func() {
				Expect(resErr).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
			})
		})

		Context("YAML", func() {
			BeforeEach(func() {
				ctype = "application/x-yaml"
				body = `foo: bar`
			})

			It("fails", func() {
				Expect(resErr).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
			})
		})
	})
})
