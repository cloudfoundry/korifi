package smoke_test

import (
	"fmt"
	"os"
	"testing"

	"code.cloudfoundry.org/korifi/tests/helpers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var (
	cfAdmin       string
	rootNamespace string
	orgName       string
)

func TestSmoke(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(helpers.EventuallyTimeout())
	SetDefaultEventuallyPollingInterval(helpers.EventuallyPollingInterval())
	RunSpecs(t, "CF CLI Tests Suite")
}

var _ = BeforeSuite(func() {
	setCFHome(GinkgoParallelProcess())

	cfAdmin = uuid.NewString()
	rootNamespace = helpers.GetDefaultedEnvVar("ROOT_NAMESPACE", "cf")
	orgName = uuid.NewString()

	serviceAccountFactory := helpers.NewServiceAccountFactory(rootNamespace)

	cfAdminToken := serviceAccountFactory.CreateAdminServiceAccount(cfAdmin)
	helpers.AddUserToKubeConfig(cfAdmin, cfAdminToken)

	Expect(helpers.Cf("api", helpers.GetRequiredEnvVar("API_SERVER_ROOT"), "--skip-ssl-validation")).To(Exit(0))
	Expect(helpers.Cf("auth", cfAdmin)).To(Exit(0))
	Expect(helpers.Cf("create-org", orgName)).To(Exit(0))
})

var _ = AfterSuite(func() {
	setCFHome(GinkgoParallelProcess())

	Expect(helpers.Cf("api", helpers.GetRequiredEnvVar("API_SERVER_ROOT"), "--skip-ssl-validation")).To(Exit(0))
	Expect(helpers.Cf("auth", cfAdmin)).To(Exit(0))
	Expect(helpers.Cf("delete-org", "-f", orgName)).To(Exit(0))

	serviceAccountFactory := helpers.NewServiceAccountFactory(rootNamespace)
	serviceAccountFactory.DeleteServiceAccount(cfAdmin)

	helpers.RemoveUserFromKubeConfig(cfAdmin)
})

var _ = BeforeEach(func() {
	setCFHome(GinkgoParallelProcess())

	Expect(helpers.Cf("api", helpers.GetRequiredEnvVar("API_SERVER_ROOT"), "--skip-ssl-validation")).To(Exit(0))
	Expect(helpers.Cf("auth", cfAdmin)).To(Exit(0))
})

func setCFHome(ginkgoNode int) {
	cfHomeDir, err := os.MkdirTemp("", fmt.Sprintf("ginkgo-%d", ginkgoNode))
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() {
		Expect(os.RemoveAll(cfHomeDir)).To(Succeed())
	})
	os.Setenv("CF_HOME", cfHomeDir)
}
