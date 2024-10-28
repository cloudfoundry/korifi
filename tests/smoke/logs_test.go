package smoke_test

import (
	"syscall"

	"code.cloudfoundry.org/korifi/tests/helpers"
	"github.com/cloudfoundry/cf-test-helpers/cf"
	"github.com/onsi/gomega/gbytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("cf logs", func() {
	Describe("cf logs --recent", func() {
		It("prints app recent logs", func() {
			Eventually(helpers.Cf("logs", sharedData.BuildpackAppName, "--recent")).Should(gbytes.Say("Listening on port 8080"))
		})
	})

	Describe("cf logs", func() {
		It("blocks waiting for new log entries", func() {
			logsSession := cf.Cf("logs", sharedData.BuildpackAppName)
			defer logsSession.Signal(syscall.SIGTERM)

			Eventually(logsSession).Should(gbytes.Say("Listening on port 8080"))
			outputLen := len(string(logsSession.Out.Contents()))

			Consistently(func(g Gomega) {
				Expect(logsSession.ExitCode()).To(Equal(-1))
				Expect(string(logsSession.Out.Contents())).To(HaveLen(outputLen))
			}).Should(Succeed())
		})
	})
})
