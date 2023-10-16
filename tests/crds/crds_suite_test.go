package crds_test

import (
	"testing"

	"code.cloudfoundry.org/korifi/tests/helpers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

func TestCrds(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(helpers.EventuallyTimeout())
	SetDefaultEventuallyPollingInterval(helpers.EventuallyPollingInterval())
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

	Expect(
		helpers.Kubectl("get", "namespace/"+rootNamespace),
	).To(Exit(0), "Could not find root namespace called %q", rootNamespace)

	cfUser = uuid.NewString()
	cfUserToken := serviceAccountFactory.CreateServiceAccount(cfUser)
	helpers.AddUserToKubeConfig(cfUser, cfUserToken)
})

var _ = AfterSuite(func() {
	serviceAccountFactory.DeleteServiceAccount(cfUser)
	helpers.RemoveUserFromKubeConfig(cfUser)
})
