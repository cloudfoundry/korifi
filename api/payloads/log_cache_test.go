package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LogCacheRead", func() {
	Describe("Validation", func() {
		DescribeTable("valid query",
			func(query string, expectedLogRead payloads.LogCacheRead) {
				actualLogRead, decodeErr := decodeQuery[payloads.LogCacheRead](query)

				Expect(decodeErr).NotTo(HaveOccurred())
				Expect(*actualLogRead).To(Equal(expectedLogRead))
			},
			Entry("start_time", "start_time=123", payloads.LogCacheRead{
				StartTime: tools.PtrTo[int64](123),
			}),
			Entry("envelope type LOG", "envelope_types=LOG", payloads.LogCacheRead{
				EnvelopeTypes: []string{"LOG"},
			}),
			Entry("envelope type GAUGE", "envelope_types=GAUGE", payloads.LogCacheRead{
				EnvelopeTypes: []string{"GAUGE"},
			}),
			Entry("limit", "limit=456", payloads.LogCacheRead{
				Limit: tools.PtrTo[int64](456),
			}),
			Entry("descending", "descending=true", payloads.LogCacheRead{
				Descending: true,
			}),
			Entry("all fields missing", "", payloads.LogCacheRead{}),
		)

		DescribeTable("invalid query",
			func(query string, expectedErrMsg string) {
				_, decodeErr := decodeQuery[payloads.LogCacheRead](query)
				Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
			},
			Entry("start_time", "start_time=foo", "invalid syntax"),
			Entry("limit", "limit=foo", "invalid syntax"),
			Entry("descending", "descending=foo", "invalid syntax"),
			Entry("envelope type", "envelope_types=foo", "value must be one of"),
		)
	})
})
