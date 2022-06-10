package workloads_test

import (
	"strconv"
	"time"

	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	"github.com/jonboulle/clockwork"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SequenceId", func() {
	var (
		clock          clockwork.FakeClock
		seqIdGenerator *workloads.SequenceId
		seqId          int64
	)

	BeforeEach(func() {
		clock = clockwork.NewFakeClockAt(time.Date(1999, time.February, 3, 4, 5, 6, 7*1000*1000, time.UTC))
		seqIdGenerator = workloads.NewSequenceId(clock)
	})

	JustBeforeEach(func() {
		var err error
		seqId, err = seqIdGenerator.Generate()
		Expect(err).NotTo(HaveOccurred())
	})

	It("generates a 17 digits sequence id", func() {
		seqIdString := strconv.FormatInt(seqId, 10)
		Expect(seqIdString).To(HaveLen(17))
	})

	It("uses the YYYYMMDDhhmmssSSS format", func() {
		Expect(seqId).To(BeNumerically("==", 19990203040506007))
	})

	When("a millisecond passes on", func() {
		BeforeEach(func() {
			clock.Advance(time.Millisecond)
		})

		It("generates a greater sequence id", func() {
			Expect(seqId).To(BeNumerically("==", 19990203040506008))
		})
	})
})
