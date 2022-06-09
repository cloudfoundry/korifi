package eirini_controller_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests/integration"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func TestEiriniController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EiriniController Suite")
}

var (
	eiriniBins     integration.EiriniBinaries
	fixture        *tests.Fixture
	configFilePath string
	session        *gexec.Session
	config         *eirinictrl.ControllerConfig
)

var _ = SynchronizedBeforeSuite(func() []byte {
	eiriniBins = integration.NewEiriniBinaries()
	eiriniBins.EiriniController.Build()

	data, err := json.Marshal(eiriniBins)
	Expect(err).NotTo(HaveOccurred())

	return data
}, func(data []byte) {
	err := json.Unmarshal(data, &eiriniBins)
	Expect(err).NotTo(HaveOccurred())

	fixture = tests.NewFixture(GinkgoWriter)

	SetDefaultEventuallyTimeout(30 * time.Second)
})

var _ = SynchronizedAfterSuite(func() {
	fixture.Destroy()
}, func() {
	eiriniBins.TearDown()
})

var _ = BeforeEach(func() {
	fixture.SetUp()
	config = integration.DefaultControllerConfig(fixture.Namespace)
})

var _ = JustBeforeEach(func() {
	session, configFilePath = eiriniBins.EiriniController.Run(config)
})

var _ = AfterEach(func() {
	Expect(os.Remove(configFilePath)).To(Succeed())
	session.Kill()

	fixture.TearDown()
})
