package tools_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"

	"code.cloudfoundry.org/korifi/tools"
)

var _ = Describe("Compare", func() {
	DescribeTable("ZeroIfNil",
		func(value *time.Time, match types.GomegaMatcher) {
			Expect(tools.ZeroIfNil(value)).To(match)
		},
		Entry("nil", nil, BeZero()),
		Entry("not nil", tools.PtrTo(time.UnixMilli(1)), Equal(time.UnixMilli(1))),
	)

	DescribeTable("CompareTimePtr",
		func(t1, t2 *time.Time, match types.GomegaMatcher) {
			Expect(tools.CompareTimePtr(t1, t2)).To(match)
		},
		Entry("nils", nil, nil, BeZero()),
		Entry("nil, not-nil", nil, tools.PtrTo(time.UnixMilli(1)), BeNumerically("<", 0)),
		Entry("not-nil, nil", tools.PtrTo(time.UnixMilli(1)), nil, BeNumerically(">", 0)),
		Entry("not-nil < not-nil", tools.PtrTo(time.UnixMilli(1)), tools.PtrTo(time.UnixMilli(2)), BeNumerically("<", 0)),
		Entry("not-nil > not-nil", tools.PtrTo(time.UnixMilli(2)), tools.PtrTo(time.UnixMilli(1)), BeNumerically(">", 0)),
		Entry("not-nil == not-nil", tools.PtrTo(time.UnixMilli(1)), tools.PtrTo(time.UnixMilli(1)), BeZero()),
	)

	DescribeTable("IfNil",
		func(v, ret *string, match types.GomegaMatcher) {
			Expect(tools.IfNil(v, ret)).To(match)
		},
		Entry("v == nil", nil, tools.PtrTo("a"), PointTo(Equal("a"))),
		Entry("v != nil", tools.PtrTo("a"), tools.PtrTo("b"), PointTo(Equal("a"))),
	)

	DescribeTable("IfZero",
		func(v, ret int, match types.GomegaMatcher) {
			Expect(tools.IfZero(v, ret)).To(match)
		},
		Entry("v is zero", 0, 1, Equal(1)),
		Entry("v is not zero", 1, 2, Equal(1)),
	)

	DescribeTable("InsertOrUpdate",
		func(key string, modifyFunc func(*string), match types.GomegaMatcher) {
			m := map[string]string{
				"foo": "bar",
			}
			tools.InsertOrUpdate(m, key, modifyFunc)
			Expect(m).To(match)
		},
		Entry("insert", "a", func(v *string) { *v = "inserted" }, Equal(map[string]string{"a": "inserted", "foo": "bar"})),
		Entry("update", "foo", func(v *string) { *v = "bar-updated" }, Equal(map[string]string{"foo": "bar-updated"})),
	)
})
