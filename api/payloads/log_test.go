package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LogRead", func() {
	Describe("Validation", func() {
		DescribeTable("valid query",
			func(query string, expectedLogRead payloads.LogRead) {
				actualLogRead, decodeErr := decodeQuery[payloads.LogRead](query)

				Expect(decodeErr).NotTo(HaveOccurred())
				Expect(*actualLogRead).To(Equal(expectedLogRead))
			},
			Entry("all fields valid", "start_time=123&envelope_types=LOG&&limit=456&descending=true", payloads.LogRead{
				StartTime:     tools.PtrTo[int64](123),
				EnvelopeTypes: []string{"LOG"},
				Limit:         tools.PtrTo[int64](456),
				Descending:    true,
			}),
			Entry("all fields missing", "", payloads.LogRead{}),
			Entry("empty descending", "descending=", payloads.LogRead{}),

			Entry("envelope type LOG", "envelope_types=LOG", payloads.LogRead{EnvelopeTypes: []string{"LOG"}}),
		)

		DescribeTable("invalid query",
			func(query string, expectedErrMsg string) {
				_, decodeErr := decodeQuery[payloads.LogRead](query)
				Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
			},
			Entry("invalid start_time", "start_time=foo", "invalid syntax"),
			Entry("invalid limit", "limit=foo", "invalid syntax"),
			Entry("invalid descending", "descending=foo", "invalid syntax"),
			Entry("invalid envelope type", "envelope_types=foo", "value must be one of"),
		)
	})
})
