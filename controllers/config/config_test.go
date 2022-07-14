package config_test

import (
	"time"

	"code.cloudfoundry.org/korifi/controllers/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseTaskTTL", func() {
	var (
		taskTTLString string
		taskTTL       time.Duration
		parseErr      error
	)

	BeforeEach(func() {
		taskTTLString = ""
	})

	JustBeforeEach(func() {
		cfg := config.ControllerConfig{
			TaskTTL: taskTTLString,
		}

		taskTTL, parseErr = cfg.ParseTaskTTL()
	})

	It("return 30 days by default", func() {
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(taskTTL).To(Equal(30 * 24 * time.Hour))
	})

	When("entering something parseable by time.ParseDuration", func() {
		BeforeEach(func() {
			taskTTLString = "12h30m5s20ns"
		})

		It("parses ok", func() {
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(taskTTL).To(Equal(12*time.Hour + 30*time.Minute + 5*time.Second + 20*time.Nanosecond))
		})
	})

	When("entering something that cannot be parsed", func() {
		BeforeEach(func() {
			taskTTLString = "foreva"
		})

		It("returns an error", func() {
			Expect(parseErr).To(HaveOccurred())
		})
	})

	When("a simple day expression", func() {
		BeforeEach(func() {
			taskTTLString = "25d"
		})

		It("parses ok", func() {
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(taskTTL).To(Equal(25 * 24 * time.Hour))
		})
	})

	When("a compound day expression", func() {
		BeforeEach(func() {
			taskTTLString = "25d13h12m"
		})

		It("parses ok", func() {
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(taskTTL).To(Equal(25*24*time.Hour + 13*time.Hour + 12*time.Minute))
		})
	})

	When("a compound day erroneous expression", func() {
		BeforeEach(func() {
			taskTTLString = "25dlater"
		})

		It("parses ok", func() {
			Expect(parseErr).To(HaveOccurred())
		})
	})

	When("it contains more than 1 'd'", func() {
		BeforeEach(func() {
			taskTTLString = "1d2d"
		})

		It("returns an error", func() {
			Expect(parseErr).To(HaveOccurred())
		})
	})
})
