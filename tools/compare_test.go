package tools_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
})
