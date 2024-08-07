package parse_test

import (
	. "code.cloudfoundry.org/korifi/api/payloads/parse"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseArrayParam", func() {
	When("an empty string is specified", func() {
		It("returns nil", func() {
			Expect(ArrayParam("")).To(BeNil())
		})
	})

	When("an single value is specified", func() {
		It("returns an array with the value specified", func() {
			Expect(ArrayParam("foo")).To(Equal([]string{"foo"}))
		})
	})

	When("multiple values are specified in a CSV", func() {
		It("returns an array with the value split on commas and all white-space removed from each value", func() {
			Expect(ArrayParam(" foo,   bar    ,   baz")).To(Equal([]string{"foo", "bar", "baz"}))
		})
	})
})
