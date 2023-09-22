package crds_test

import (
	"testing"
	"time"

	"code.cloudfoundry.org/korifi/tests/helpers"

	"github.com/cloudfoundry/cf-test-helpers/cf"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

func TestCrds(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(10 * time.Second)
	RunSpecs(t, "CRDs Suite")
}

var (
	rootNamespace         string
	serviceAccountFactory *helpers.ServiceAccountFactory
	cfUser                string
)

var _ = BeforeSuite(func() {
	rootNamespace = helpers.GetDefaultedEnvVar("ROOT_NAMESPACE", "cf")
	serviceAccountFactory = helpers.NewServiceAccountFactory(rootNamespace)

	Eventually(
		helpers.Kubectl("get", "namespace/"+rootNamespace),
	).Should(Exit(0), "Could not find root namespace called %q", rootNamespace)

	cfUser = uuid.NewString()
	cfUserToken := serviceAccountFactory.CreateServiceAccount(cfUser)
	helpers.AddUserToKubeConfig(cfUser, cfUserToken)
})

var _ = AfterSuite(func() {
	serviceAccountFactory.DeleteServiceAccount(cfUser)
	helpers.RemoveUserFromKubeConfig(cfUser)
})

func loginAs(apiEndpoint string, user string) {
	apiArguments := []string{
		"api",
		apiEndpoint,
		"--skip-ssl-validation",
	}
	Eventually(cf.Cf(apiArguments...)).Should(Exit(0))

	Eventually(cf.Cf("auth", user)).Should(Exit(0))
}
