package payloads_test

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LogRead", func() {
	DescribeTable("DecodeFromURLValues",
		func(query string, logRead payloads.LogRead, err string) {
			actualLogRead := payloads.LogRead{}
			values, parseErr := url.ParseQuery(query)
			Expect(parseErr).NotTo(HaveOccurred())

			decodeErr := actualLogRead.DecodeFromURLValues(values)

			if err == "" {
				Expect(decodeErr).NotTo(HaveOccurred())
				Expect(actualLogRead).To(Equal(logRead))
			} else {
				Expect(decodeErr).To(MatchError(ContainSubstring(err)))
			}
		},
		Entry("all fields valid", "start_time=123&envelope_types=one&envelope_types=two&limit=456&descending=true", payloads.LogRead{
			StartTime:     123,
			EnvelopeTypes: []string{"one", "two"},
			Limit:         456,
			Descending:    true,
		}, ""),
		Entry("all fields missing", "", payloads.LogRead{}, ""),
		Entry("invalid start_time", "start_time=foo", payloads.LogRead{}, "invalid syntax"),
		Entry("empty start_time", "start_time=", payloads.LogRead{}, ""),
		Entry("invalid limit", "limit=foo", payloads.LogRead{}, "invalid syntax"),
		Entry("empty limit", "limit=", payloads.LogRead{}, ""),
		Entry("invalid descending", "descending=foo", payloads.LogRead{}, "invalid syntax"),
		Entry("empty descending", "descending=", payloads.LogRead{}, ""),
	)
})
