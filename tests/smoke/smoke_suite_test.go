package smoke_test

import (
	"os"
	"testing"
	"time"

	"github.com/cloudfoundry/cf-test-helpers/cf"
	"github.com/cloudfoundry/cf-test-helpers/generator"
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

const NamePrefix = "cf-on-k8s-smoke"

var (
	orgName          string
	spaceName        string
	appName          string
	appsDomain       string
	appRouteProtocol string
)

func TestSmoke(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(10 * time.Minute)
	RunSpecs(t, "Smoke Tests Suite")
}

var _ = BeforeSuite(func() {
	apiArguments := []string{"api", GetRequiredEnvVar("SMOKE_TEST_API_ENDPOINT")}
	skipSSL := os.Getenv("SMOKE_TEST_SKIP_SSL") == "true"
	if skipSSL {
		apiArguments = append(apiArguments, "--skip-ssl-validation")
	}

	Eventually(cf.Cf(apiArguments...)).Should(Exit(0))

	loginAs(GetRequiredEnvVar("SMOKE_TEST_USER"))

	appRouteProtocol = GetDefaultedEnvVar("SMOKE_TEST_APP_ROUTE_PROTOCOL", "https")
	appsDomain = GetRequiredEnvVar("SMOKE_TEST_APPS_DOMAIN")
	orgName = generator.PrefixedRandomName(NamePrefix, "org")
	spaceName = generator.PrefixedRandomName(NamePrefix, "space")
	appName = generator.PrefixedRandomName(NamePrefix, "app")

	Eventually(cf.Cf("create-org", orgName)).Should(Exit(0))
	Eventually(cf.Cf("create-space", "-o", orgName, spaceName)).Should(Exit(0))
	Eventually(cf.Cf("target", "-o", orgName, "-s", spaceName)).Should(Exit(0))

	Eventually(
		cf.Cf("push", appName, "-p", "../assets/dorifi"),
	).Should(Exit(0))
})

var _ = AfterSuite(func() {
	if CurrentSpecReport().State.Is(types.SpecStateFailed) {
		printAppReport(appName)
	}

	if orgName != "" {
		Eventually(func() *Session {
			return cf.Cf("delete-org", orgName, "-f").Wait()
		}, 2*time.Minute, 1*time.Second).Should(Exit(0))
	}
})

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

func getSpaceName() string {
	return spaceName
}
