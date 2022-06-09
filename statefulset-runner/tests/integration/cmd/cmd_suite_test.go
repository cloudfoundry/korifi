package cmd_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests/integration"
)

var (
	fixture    *tests.Fixture
	eiriniBins integration.EiriniBinaries
	testEnv    *envtest.Environment
)

var _ = SynchronizedBeforeSuite(func() []byte {
	eiriniBins = integration.NewEiriniBinaries()

	data, err := json.Marshal(eiriniBins)
	Expect(err).NotTo(HaveOccurred())

	return data
}, func(data []byte) {
	err := json.Unmarshal(data, &eiriniBins)
	Expect(err).NotTo(HaveOccurred())
	SetDefaultConsistentlyDuration(time.Second * 2)

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "deployment", "helm", "templates", "core", "lrp-crd.yml"),
			filepath.Join("..", "..", "..", "deployment", "helm", "templates", "core", "task-crd.yml"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	fixture = tests.NewFixture(cfg, GinkgoWriter)
})

var _ = SynchronizedAfterSuite(func() {
	fixture.Destroy()
}, func() {
	eiriniBins.TearDown()
	Expect(testEnv.Stop()).To(Succeed())
})

var _ = BeforeEach(func() {
	fixture.SetUp()
})

var _ = AfterEach(func() {
	fixture.TearDown()
})

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cmd Suite")
}
