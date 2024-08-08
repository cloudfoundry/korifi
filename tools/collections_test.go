package tools_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	"code.cloudfoundry.org/korifi/tools"
)

var _ = Describe("Collections", func() {
	Describe("Uniq", func() {
		It("deduplicates the slice", func() {
			Expect(tools.Uniq([]string{"b", "a", "b", "a"})).To(ConsistOf("a", "b"))
		})
	})

	DescribeTable("EmptyOrContains",
		func(slice []string, match types.GomegaMatcher) {
			Expect(tools.EmptyOrContains(slice, "needle")).To(match)
		},
		Entry("empty slice", []string{}, BeTrue()),
		Entry("contains element", []string{"nail", "needle", "pin"}, BeTrue()),
		Entry("does not contain element", []string{"nail", "pin"}, BeFalse()),
	)

	DescribeTable("NilOrEquals",
		func(value *string, match types.GomegaMatcher) {
			Expect(tools.NilOrEquals(value, "needle")).To(match)
		},
		Entry("nil", nil, BeTrue()),
		Entry("equal", tools.PtrTo("needle"), BeTrue()),
		Entry("not-equal", tools.PtrTo("not-needle"), BeFalse()),
	)
})
