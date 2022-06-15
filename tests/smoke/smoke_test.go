package smoke_test

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/cloudfoundry/cf-test-helpers/cf"
	"github.com/cloudfoundry/cf-test-helpers/generator"
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
)

const NamePrefix = "cf-on-k8s-smoke"

func GetRequiredEnvVar(envVarName string) string {
	value, ok := os.LookupEnv(envVarName)
	Expect(ok).To(BeTrue(), envVarName+" environment variable is required, but was not provided.")
	return value
}

func GetDefaultedEnvVar(envVarName, defaultValue string) string {
	value, ok := os.LookupEnv(envVarName)
	if !ok {
		return defaultValue
	}
	return value
}

var _ = Describe("Smoke Tests", func() {
	When("running cf push", func() {
		var (
			orgName                      string
			appName                      string
			appsDomain, appRouteProtocol string
		)

		BeforeEach(func() {
			apiArguments := []string{"api", GetRequiredEnvVar("SMOKE_TEST_API_ENDPOINT")}
			skipSSL := os.Getenv("SMOKE_TEST_SKIP_SSL") == "true"
			if skipSSL {
				apiArguments = append(apiArguments, "--skip-ssl-validation")
			}

			cfAPI := cf.Cf(apiArguments...)
			Eventually(cfAPI).Should(Exit(0))

			loginAs(GetRequiredEnvVar("SMOKE_TEST_USER"))

			appRouteProtocol = GetDefaultedEnvVar("SMOKE_TEST_APP_ROUTE_PROTOCOL", "https")
			appsDomain = GetRequiredEnvVar("SMOKE_TEST_APPS_DOMAIN")
			orgName = generator.PrefixedRandomName(NamePrefix, "org")
			spaceName := generator.PrefixedRandomName(NamePrefix, "space")

			Eventually(cf.Cf("create-org", orgName)).Should(Exit(0))
			Eventually(cf.Cf("create-space", "-o", orgName, spaceName)).Should(Exit(0))
			Eventually(cf.Cf("target", "-o", orgName, "-s", spaceName)).Should(Exit(0))
		})

		AfterEach(func() {
			if CurrentSpecReport().State.Is(types.SpecStateFailed) {
				printAppReport(appName)
			}

			if orgName != "" {
				Eventually(func() *Session {
					return cf.Cf("delete-org", orgName, "-f").Wait()
				}, 2*time.Minute, 1*time.Second).Should(Exit(0))
			}
		})

		It("creates a routable app pod in Kubernetes from a source-based app", func() {
			appName = generator.PrefixedRandomName(NamePrefix, "app")

			cfPush := cf.Cf("push", appName, "-p", "assets/test-node-app")
			Eventually(cfPush).Should(Exit(0))

			var httpClient http.Client
			httpClient.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}

			Eventually(func(g Gomega) {
				resp, err := httpClient.Get(fmt.Sprintf("%s://%s.%s", appRouteProtocol, appName, appsDomain))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp).To(HaveHTTPStatus(http.StatusOK))
				g.Expect(resp).To(HaveHTTPBody(ContainSubstring("Hello World")))
			}, 2*time.Minute, 30*time.Second).Should(Succeed())
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
	r, w := io.Pipe()
	loginSession := cf.CfWithStdin(r, "login")
	Eventually(loginSession).Should(Say("Choose your Kubernetes"))

	userEntryRegex, err := regexp.Compile(`(\d*). ` + user + `\s`)
	Expect(err).NotTo(HaveOccurred())
	matches := userEntryRegex.FindSubmatch(loginSession.Buffer().Contents())
	Expect(matches).To(HaveLen(2))

	Expect(fmt.Fprintf(w, "%s\n\n", matches[1])).To(BeNumerically(">=", 3))
	Expect(w.Close()).To(Succeed())

	Eventually(loginSession).Should(Exit(0))
}
