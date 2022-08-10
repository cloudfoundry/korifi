package tools_test

import (
	"time"

	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseDuration", func() {
	var (
		durationString string
		duration       time.Duration
		parseErr       error
	)

	BeforeEach(func() {
		durationString = "12h30m5s20ns"
	})

	JustBeforeEach(func() {
		duration, parseErr = tools.ParseDuration(durationString)
	})

	It("parses ok", func() {
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(duration).To(Equal(12*time.Hour + 30*time.Minute + 5*time.Second + 20*time.Nanosecond))
	})

	When("entering something that cannot be parsed", func() {
		BeforeEach(func() {
			durationString = "foreva"
		})

		It("returns an error", func() {
			Expect(parseErr).To(HaveOccurred())
		})
	})

	When("a simple day expression", func() {
		BeforeEach(func() {
			durationString = "25d"
		})

		It("parses ok", func() {
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(duration).To(Equal(25 * 24 * time.Hour))
		})
	})

	When("a compound day expression", func() {
		BeforeEach(func() {
			durationString = "25d13h12m"
		})

		It("parses ok", func() {
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(duration).To(Equal(25*24*time.Hour + 13*time.Hour + 12*time.Minute))
		})
	})

	When("a compound day erroneous expression", func() {
		BeforeEach(func() {
			durationString = "25dlater"
		})

		It("parses ok", func() {
			Expect(parseErr).To(HaveOccurred())
		})
	})

	When("it contains more than 1 'd'", func() {
		BeforeEach(func() {
			durationString = "1d2d"
		})

		It("returns an error", func() {
			Expect(parseErr).To(HaveOccurred())
		})
	})
})
