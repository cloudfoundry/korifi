package tools_test

import (
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EncodeValueSha224", func() {
	It("encodes the value", func() {
		Expect(tools.EncodeValueToSha224("hello")).To(
			Equal("ea09ae9cc6768c50fcee903ed054556e5bfc8347907f12598aa24193"),
		)
	})
})

var _ = Describe("EncodeValuesSha224", func() {
	It("encodes the values", func() {
		Expect(tools.EncodeValuesToSha224("hello", "world")).To(ConsistOf(
			"ea09ae9cc6768c50fcee903ed054556e5bfc8347907f12598aa24193",
			"06d2dbdb71973e31e4f1df3d7001fa7de268aa72fcb1f6f9ea37e0e5",
		))
	})
})
