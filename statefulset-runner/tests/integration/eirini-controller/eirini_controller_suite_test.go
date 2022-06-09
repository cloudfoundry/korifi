package eirini_controller_test

import (
	"context"
	"path/filepath"
	"testing"

	"code.cloudfoundry.org/lager"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/cmd/wiring"
	eirinischeme "code.cloudfoundry.org/korifi/statefulset-runner/pkg/generated/clientset/versioned/scheme"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests/integration"
)

func TestEiriniController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EiriniController Suite")
}

var (
	config  *eirinictrl.ControllerConfig
	fixture *tests.Fixture
	cancel  context.CancelFunc
	testEnv *envtest.Environment
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancelFunc := context.WithCancel(context.TODO())
	cancel = cancelFunc

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

	Expect(eirinischeme.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme.Scheme)).To(Succeed())

	fixture = tests.NewFixture(cfg, GinkgoWriter)
	//fixture.SetUp()

	// start webhook server using Manager
	webhookInstallOptions := &testEnv.WebhookInstallOptions
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		Host:               webhookInstallOptions.LocalServingHost,
		Port:               webhookInstallOptions.LocalServingPort,
		CertDir:            webhookInstallOptions.LocalServingCertDir,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	Expect(err).NotTo(HaveOccurred())

	lagerLogger := lager.NewLogger("eirini-controller")
	lagerLogger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))

	config = integration.DefaultControllerConfig(fixture.Namespace)

	Expect(wiring.LRPReconciler(lagerLogger, mgr, *config)).To(Succeed())
	Expect(wiring.TaskReconciler(lagerLogger, mgr, *config)).To(Succeed())

	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	}()
})

var _ = AfterSuite(func() {
	fixture.Destroy()
	cancel() // call the cancel function to stop the controller context
	Expect(testEnv.Stop()).To(Succeed())
})

var _ = BeforeEach(func() {
	fixture.SetUp()
	config = integration.DefaultControllerConfig(fixture.Namespace)
})

var _ = AfterEach(func() {
	fixture.TearDown()
})
