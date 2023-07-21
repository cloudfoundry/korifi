package smoke_test

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

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
	Describe("cf push", func() {
		It("runs the app", func() {
			appResponseShould("/", SatisfyAll(
				HaveHTTPStatus(http.StatusOK),
				HaveHTTPBody(ContainSubstring("Hi, I'm Dorifi!")),
			))
		})
	})

	Describe("cf logs", func() {
		It("prints app logs", func() {
			Eventually(func(g Gomega) {
				cfLogs := cf.Cf("logs", appName, "--recent")
				g.Expect(cfLogs.Wait().Out).To(gbytes.Say("Listening on port 8080"))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())
		})
	})

	Describe("cf run-task", func() {
		It("succeeds", func() {
			cfRunTask := cf.Cf("run-task", appName, "-c", `echo "Hello from the task"`)
			Eventually(cfRunTask).Should(Exit(0))
		})
	})

	Describe("cf bind-service", func() {
		BeforeEach(func() {
			serviceName := generator.PrefixedRandomName(NamePrefix, "svc")

			Eventually(
				cf.Cf("create-user-provided-service", serviceName, "-p", `{"key1":"value1","key2":"value2"}`),
			).Should(Exit(0))

			Eventually(cf.Cf("bind-service", appName, serviceName)).Should(Exit(0))
			Eventually(cf.Cf("restart", appName)).Should(Exit(0))
		})

		It("binds the service to the app", func() {
			appResponseShould("/env.json", SatisfyAll(
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

func printAppReport(appName string) {
	if appName == "" {
		return
	}

	printAppReportBanner(fmt.Sprintf("***** APP REPORT: %s *****", appName))
	Eventually(cf.Cf("app", appName, "--guid")).Should(Exit())
	Eventually(cf.Cf("logs", "--recent", appName)).Should(Exit())
	printAppReportBanner(fmt.Sprintf("*** END APP REPORT: %s ***", appName))
}

func printAppReportBanner(announcement string) {
	sequence := strings.Repeat("*", len(announcement))
	fmt.Fprintf(GinkgoWriter, "\n\n%s\n%s\n%s\n", sequence, announcement, sequence)
}

func loginAs(user string) {
	// Stdin contains username followed by 2 return carriages. Firtst one
	// enters the username and second one skips the org selection prompt that
	// is presented if there is more than one org
	loginSession := cf.CfWithStdin(bytes.NewBufferString(user+"\n\n"), "login")
	Eventually(loginSession).Should(Exit(0))
}

func appResponseShould(requestPath string, matchExpectations types.GomegaMatcher) {
	var httpClient http.Client
	httpClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	Eventually(func(g Gomega) {
		resp, err := httpClient.Get(fmt.Sprintf("%s://%s.%s%s", appRouteProtocol, appName, appsDomain, requestPath))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(resp).To(matchExpectations)
	}, 5*time.Minute, 30*time.Second).Should(Succeed())
}
