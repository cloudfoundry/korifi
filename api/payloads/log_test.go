package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
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
			Entry("all fields valid", "start_time=123&envelope_types=LOG&envelope_types=COUNTER&limit=456&descending=true", payloads.LogRead{
				StartTime:     123,
				EnvelopeTypes: []string{"LOG", "COUNTER"},
				Limit:         456,
				Descending:    true,
			}),
			Entry("all fields missing", "", payloads.LogRead{}),
			Entry("empty start_time", "start_time=", payloads.LogRead{}),
			Entry("empty end_time", "end_time=", payloads.LogRead{}),
			Entry("empty limit", "limit=", payloads.LogRead{}),
			Entry("empty descending", "descending=", payloads.LogRead{}),

			Entry("envelope type LOG", "envelope_types=LOG", payloads.LogRead{EnvelopeTypes: []string{"LOG"}}),
			Entry("envelope type COUNTER", "envelope_types=COUNTER", payloads.LogRead{EnvelopeTypes: []string{"COUNTER"}}),
			Entry("envelope type GAUGE", "envelope_types=GAUGE", payloads.LogRead{EnvelopeTypes: []string{"GAUGE"}}),
			Entry("envelope type TIMER", "envelope_types=TIMER", payloads.LogRead{EnvelopeTypes: []string{"TIMER"}}),
			Entry("envelope type EVENT", "envelope_types=EVENT", payloads.LogRead{EnvelopeTypes: []string{"EVENT"}}),
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
