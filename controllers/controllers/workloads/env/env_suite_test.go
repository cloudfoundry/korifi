package env_test

import (
	"context"
	"path/filepath"
	"testing"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestEnv(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Env Suite")
}

var (
	ctx              context.Context
	k8sManagerCancel context.CancelFunc
	testEnv          *envtest.Environment
	k8sClient        client.Client
	namespace        string
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))

	ctx = context.Background()

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "helm", "controllers", "templates", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme.Scheme)).To(Succeed())

	webhookInstallOptions := &testEnv.WebhookInstallOptions
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		Host:               webhookInstallOptions.LocalServingHost,
		Port:               webhookInstallOptions.LocalServingPort,
		CertDir:            webhookInstallOptions.LocalServingCertDir,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(shared.SetupIndexWithManager(k8sManager)).To(Succeed())

	k8sClient = k8sManager.GetClient()

	k8sManagerCtx, cancelFunc := context.WithCancel(context.TODO())
	k8sManagerCancel = cancelFunc

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(k8sManagerCtx)
		Expect(err).NotTo(HaveOccurred())
	}()
})

var _ = BeforeEach(func() {
	namespace = testutils.PrefixedGUID("test-namespace")
	Expect(k8sClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	})).To(Succeed())
})

var _ = AfterSuite(func() {
	k8sManagerCancel()
	Expect(testEnv.Stop()).To(Succeed())
})
