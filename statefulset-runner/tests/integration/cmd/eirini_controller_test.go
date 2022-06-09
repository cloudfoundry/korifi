package cmd_test

import (
	"fmt"
	"net"
	"os"

	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests/integration"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("EiriniController", func() {
	var (
		config         *eirinictrl.ControllerConfig
		configFilePath string
		session        *gexec.Session
	)

	BeforeEach(func() {
		config = integration.DefaultControllerConfig(fixture.Namespace)
	})

	JustBeforeEach(func() {
		session, configFilePath = eiriniBins.EiriniController.Run(config)
	})

	AfterEach(func() {
		if configFilePath != "" {
			Expect(os.Remove(configFilePath)).To(Succeed())
		}
		if session != nil {
			Eventually(session.Kill()).Should(gexec.Exit())
		}
	})

	It("should be able to start properly", func() {
		Consistently(session).ShouldNot(gexec.Exit())
	})

	When("the config file doesn't exist", func() {
		It("exits reporting missing config file", func() {
			session = eiriniBins.EiriniController.Restart("/does/not/exist", session)
			Eventually(session).Should(gexec.Exit())
			Expect(session.ExitCode).ToNot(BeZero())
			Expect(session.Err).To(gbytes.Say("Failed to read config file: failed to read file"))
		})
	})

	Describe("Webhooks", func() {
		It("runs the webhook service and registers it", func() {
			Eventually(func() error {
				_, err := net.Dial("tcp", fmt.Sprintf(":%d", config.WebhookPort))

				return err
			}).Should(Succeed())

			Consistently(session).ShouldNot(gexec.Exit())
		})
	})
})
