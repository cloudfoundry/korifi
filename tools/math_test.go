package tools_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/tools"
)

var _ = Describe("Math", func() {
	Describe("Min", func() {
		It("returns the lesser value", func() {
			Expect(tools.Min(5, 6)).To(Equal(5))
		})
	})

	Describe("Max", func() {
		It("returns the greater value", func() {
			Expect(tools.Max(5, 6)).To(Equal(6))
		})
	})
})
