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

	DescribeTable("ZeroOrEquals",
		func(value string, match types.GomegaMatcher) {
			Expect(tools.ZeroOrEquals(value, "needle")).To(match)
		},
		Entry("zero value", "", BeTrue()),
		Entry("equal", "needle", BeTrue()),
		Entry("not-equal", "not-needle", BeFalse()),
	)

	DescribeTable("SetMapValue",
		func(m map[string]int, key string, value int, expected map[string]int) {
			result := tools.SetMapValue(m, key, value)
			Expect(result).To(Equal(expected))
		},
		Entry("Set value in non-nil map", map[string]int{"a": 1}, "b", 2, map[string]int{"a": 1, "b": 2}),
		Entry("Set value in nil map", nil, "a", 1, map[string]int{"a": 1}),
		Entry("Update existing key in map", map[string]int{"a": 1}, "a", 2, map[string]int{"a": 2}),
	)

	DescribeTable("GetMapValue",
		func(m map[string]int, key string, expected int) {
			result := tools.GetMapValue(m, key, -1)
			Expect(result).To(Equal(expected))
		},
		Entry("Value is present", map[string]int{"a": 1}, "a", 1),
		Entry("Value is missing", map[string]int{}, "a", -1),
	)
})
