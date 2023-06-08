package finalizer_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/version"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers/webhooks/finalizer"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

var (
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	k8sClient client.Client
)

const (
	rootNamespace = "cf"
)

func TestWorkloadsWebhooks(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Workloads Validating Webhooks Integration Test Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancelFunc := context.WithCancel(context.TODO())
	cancel = cancelFunc

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "helm", "korifi", "controllers", "crds"),
			filepath.Join("..", "..", "..", "..", "tests", "vendor", "kpack"),
		},
		ErrorIfCRDPathMissing: true,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "..", "helm", "korifi", "controllers", "manifests.yaml"),
				filepath.Join("..", "..", "..", "..", "helm", "korifi", "kpack-image-builder", "manifests.yaml"),
			},
		},
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	scheme := runtime.NewScheme()
	Expect(korifiv1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(admissionv1beta1.AddToScheme(scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
	Expect(buildv1alpha2.AddToScheme(scheme)).To(Succeed())

	webhookInstallOptions := &testEnv.WebhookInstallOptions
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		Host:               webhookInstallOptions.LocalServingHost,
		Port:               webhookInstallOptions.LocalServingPort,
		CertDir:            webhookInstallOptions.LocalServingCertDir,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	Expect(err).NotTo(HaveOccurred())

	k8sClient = mgr.GetClient()

	finalizer.NewKpackImageBuilderFinalizerWebhook().SetupWebhookWithManager(mgr)
	version.NewVersionWebhook("some-version").SetupWebhookWithManager(mgr)

	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	}()

	// wait for the webhook server to get ready
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	Eventually(func() error {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	}).Should(Succeed())

	// Create root namespace
	Expect(k8sClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: rootNamespace,
		},
	})).To(Succeed())
})

var _ = AfterSuite(func() {
	cancel() // call the cancel function to stop the controller context
	Expect(testEnv.Stop()).To(Succeed())
})
