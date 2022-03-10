package smoke_test

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"

	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	"github.com/cloudfoundry-incubator/cf-test-helpers/generator"
)

const NamePrefix = "cf-on-k8s-smoke"

func GetRequiredEnvVar(envVarName string) string {
	value, ok := os.LookupEnv(envVarName)
	if !ok {
		panic(envVarName + " environment variable is required, but was not provided.")
	}
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
			doLogin := GetDefaultedEnvVar("SMOKE_TEST_LOGIN", "")

			if doLogin != "" {
				apiEndpoint := GetRequiredEnvVar("SMOKE_TEST_API_ENDPOINT")
				// username := GetRequiredEnvVar("SMOKE_TEST_USERNAME")
				// password := GetRequiredEnvVar("SMOKE_TEST_PASSWORD")

				apiArguments := []string{"api", apiEndpoint}
				skip_ssl, _ := os.LookupEnv("SMOKE_TEST_SKIP_SSL")
				skip_ssl_bool := skip_ssl == "true"
				if skip_ssl_bool {
					apiArguments = append(apiArguments, "--skip-ssl-validation")
				}
				http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: skip_ssl_bool}

				// Target CF and auth
				cfAPI := cf.Cf(apiArguments...)
				Eventually(cfAPI).Should(Exit(0))

				// Authenticate - untested not sure if this works for cf-on-k8s
				Eventually(cf.Cf("login").Wait()).Should(Exit(0))
			}

			appRouteProtocol = GetDefaultedEnvVar("SMOKE_TEST_APP_ROUTE_PROTOCOL", "https")
			appsDomain = GetRequiredEnvVar("SMOKE_TEST_APPS_DOMAIN")
			// Create an org and space and target
			orgName = generator.PrefixedRandomName(NamePrefix, "org")
			spaceName := generator.PrefixedRandomName(NamePrefix, "space")

			Eventually(cf.Cf("create-org", orgName)).Should(Exit(0))
			Eventually(cf.Cf("create-space", "-o", orgName, spaceName)).Should(Exit(0))
			Eventually(cf.Cf("target", "-o", orgName, "-s", spaceName)).Should(Exit(0))
		})

		AfterEach(func() {
			if CurrentGinkgoTestDescription().Failed {
				printAppReport(appName)
			}

			if orgName != "" {
				// Delete the test org
				Eventually(func() *Session {
					return cf.Cf("delete-org", orgName, "-f").Wait()
				}, 2*time.Minute, 1*time.Second).Should(Exit(0))
			}
		})

		It("creates a routable app pod in Kubernetes from a source-based app", func() {
			appName = generator.PrefixedRandomName(NamePrefix, "app")

			By("pushing an app and checking that the CF CLI command succeeds")
			cfPush := cf.Cf("push", appName, "-p", "assets/test-node-app")
			Eventually(cfPush).Should(Exit(0))

			By("querying the app")
			var resp *http.Response

			Eventually(func() int {
				var err error
				http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
				resp, err = http.Get(fmt.Sprintf("%s://%s.%s", appRouteProtocol, appName, appsDomain))
				Expect(err).NotTo(HaveOccurred())
				return resp.StatusCode
			}, 2*time.Minute, 30*time.Second).Should(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("Hello World\n"))

			// TODO: Log retrieval is not supported yet
			//By("verifying that the application's logs are available.")
			//Eventually(func() string {
			//	cfLogs := cf.Cf("logs", appName, "--recent")
			//	return string(cfLogs.Wait().Out.Contents())
			//}, 2*time.Minute, 2*time.Second).Should(ContainSubstring("Console output from test-node-app"))
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
