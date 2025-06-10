package parse_test

import (
	"code.cloudfoundry.org/korifi/api/payloads/parse"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseInteger", func() {
	When("a non-integer string is specified", func() {
		It("returns zero", func() {
			Expect(parse.Integer("abc")).To(BeZero())
		})
	})

	When("a string representing an integer is specified", func() {
		It("returns an array with the value specified", func() {
			Expect(parse.Integer("5")).To(Equal(5))
		})
	})
})
