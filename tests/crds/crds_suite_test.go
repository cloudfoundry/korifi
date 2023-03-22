package crds_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloudfoundry/cf-test-helpers/cf"
	"github.com/cloudfoundry/cf-test-helpers/commandreporter"
	"github.com/cloudfoundry/cf-test-helpers/commandstarter"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

func TestCrds(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(10 * time.Second)
	RunSpecs(t, "CRDs Suite")
}

var rootNamespace string

var _ = BeforeSuite(func() {
	rootNamespace = GetDefaultedEnvVar("ROOT_NAMESPACE", "cf")
	Eventually(
		kubectl("get", "namespace/"+rootNamespace),
	).Should(Exit(0), "Could not find root namespace called %q", rootNamespace)
})

func kubectl(args ...string) *Session {
	cmdStarter := commandstarter.NewCommandStarter()
	return kubectlWithCustomReporter(cmdStarter, commandreporter.NewCommandReporter(), args...)
}

func kubectlApply(stdinText string, sprintfArgs ...any) *Session {
	cmdStarter := commandstarter.NewCommandStarterWithStdin(
		strings.NewReader(
			fmt.Sprintf(stdinText, sprintfArgs...),
		),
	)
	return kubectlWithCustomReporter(cmdStarter, commandreporter.NewCommandReporter(), "apply", "-f=-")
}

func kubectlWithCustomReporter(cmdStarter *commandstarter.CommandStarter, reporter *commandreporter.CommandReporter, args ...string) *Session {
	request, err := cmdStarter.Start(reporter, "kubectl", args...)
	if err != nil {
		panic(err)
	}

	return request
}

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

func loginAs(apiEndpoint string, skipSSL bool, user string) {
	apiArguments := []string{"api", apiEndpoint}
	if skipSSL {
		apiArguments = append(apiArguments, "--skip-ssl-validation")
	}

	Eventually(cf.Cf(apiArguments...)).Should(Exit(0))
	Eventually(cf.Cf("auth", user)).Should(Exit(0))
}
