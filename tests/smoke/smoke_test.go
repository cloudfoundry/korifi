package smoke_test

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"syscall"

	"code.cloudfoundry.org/korifi/tests/helpers"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	"github.com/cloudfoundry/cf-test-helpers/cf"
	"github.com/cloudfoundry/cf-test-helpers/generator"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/types"
)

var _ = Describe("Smoke Tests", func() {
	Describe("apps", func() {
		It("buildpack app is reachable via its route", func() {
			appResponseShould(buildpackAppName, "/", SatisfyAll(
				HaveHTTPStatus(http.StatusOK),
				HaveHTTPBody(ContainSubstring("Hi, I'm Dorifi!")),
			))
		})

		It("docker app is reachable via its route", func() {
			appResponseShould(dockerAppName, "/", SatisfyAll(
				HaveHTTPStatus(http.StatusOK),
				HaveHTTPBody(ContainSubstring("Hi, I'm not Dora!")),
			))
		})
	})

	Describe("cf logs --recent", func() {
		It("prints app recent logs", func() {
			Eventually(helpers.Cf("logs", buildpackAppName, "--recent")).Should(gbytes.Say("Listening on port 8080"))
		})
	})

	Describe("cf logs", func() {
		It("blocks waiting for new log entries", func() {
			logsSession := cf.Cf("logs", buildpackAppName)
			defer logsSession.Signal(syscall.SIGQUIT)

			Eventually(logsSession).Should(gbytes.Say("Listening on port 8080"))
			outputLen := len(string(logsSession.Out.Contents()))

			Consistently(func(g Gomega) {
				Expect(logsSession.ExitCode()).To(Equal(-1))
				Expect(string(logsSession.Out.Contents())).To(HaveLen(outputLen))
			}).Should(Succeed())
		})
	})

	Describe("cf run-task", func() {
		It("succeeds", func() {
			Eventually(helpers.Cf("run-task", buildpackAppName, "-c", `echo "Hello from the task"`)).Should(Exit(0))
		})
	})

	Describe("cf bind-service", func() {
		BeforeEach(func() {
			serviceName := generator.PrefixedRandomName(NamePrefix, "svc")

			Expect(
				helpers.Cf("create-user-provided-service", serviceName, "-p", `{"key1":"value1","key2":"value2"}`),
			).To(Exit(0))

			Expect(helpers.Cf("bind-service", buildpackAppName, serviceName)).To(Exit(0))
			Expect(helpers.Cf("restart", buildpackAppName)).To(Exit(0))
		})

		It("binds the service to the app", func() {
			appResponseShould(buildpackAppName, "/env.json", SatisfyAll(
				HaveHTTPStatus(http.StatusOK),
				HaveHTTPBody(
					MatchJSONPath("$.VCAP_SERVICES", SatisfyAll(
						MatchJSONPath(`$["user-provided"][0].credentials.key1`, "value1"),
						MatchJSONPath(`$["user-provided"][0].credentials.key2`, "value2"),
					)),
				),
			))
		})
	})
})

func appResponseShould(appName, requestPath string, matchExpectations types.GomegaMatcher) {
	var httpClient http.Client
	httpClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	Eventually(func(g Gomega) {
		resp, err := httpClient.Get(fmt.Sprintf("https://%s.%s%s", appName, appsDomain, requestPath))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(resp).To(matchExpectations)
	}).Should(Succeed())
}
