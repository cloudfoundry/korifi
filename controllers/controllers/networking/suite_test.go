package networking_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	. "code.cloudfoundry.org/korifi/controllers/controllers/networking"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/coordination"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/controllers/webhooks/networking"
	"code.cloudfoundry.org/korifi/controllers/webhooks/version"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"
	"code.cloudfoundry.org/korifi/tests/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

const rootNamespace = "cf"

var (
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	k8sClient client.Client
)

func TestNetworkingControllers(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Networking Controllers Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	var ctx context.Context
	ctx, cancel = context.WithCancel(context.TODO())

	// TODO: Add directory path for Contour CRDs
	testEnv = &envtest.Environment{
		// TODO: Reconcile with CRDInstallOptions
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "helm", "korifi", "controllers", "crds"),
			filepath.Join("..", "..", "..", "tests", "vendor", "contour"),
		},
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "..", "helm", "korifi", "controllers", "manifests.yaml")},
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(contourv1.AddToScheme(scheme.Scheme)).To(Succeed())

	webhookInstallOptions := &testEnv.WebhookInstallOptions
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		Host:               webhookInstallOptions.LocalServingHost,
		Port:               webhookInstallOptions.LocalServingPort,
		CertDir:            webhookInstallOptions.LocalServingCertDir,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	Expect(err).ToNot(HaveOccurred())

	k8sClient = helpers.NewCacheSyncingClient(k8sManager.GetClient())

	err = (NewCFRouteReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		ctrl.Log.WithName("controllers").WithName("CFRoute"),
		&config.ControllerConfig{
			CFProcessDefaults: config.CFProcessDefaults{
				MemoryMB:    500,
				DiskQuotaMB: 512,
			},
			WorkloadsTLSSecretName:      "korifi-workloads-ingress-cert",
			WorkloadsTLSSecretNamespace: "korifi-controllers-system",
		},
	)).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (NewCFDomainReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		ctrl.Log.WithName("controllers").WithName("CFDomain"),
	)).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	finalizer.NewControllersFinalizerWebhook().SetupWebhookWithManager(k8sManager)
	version.NewVersionWebhook("some-version").SetupWebhookWithManager(k8sManager)
	Expect((&korifiv1alpha1.CFApp{}).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect(workloads.NewCFAppValidator(
		webhooks.NewDuplicateValidator(coordination.NewNameRegistry(k8sManager.GetClient(), workloads.AppEntityType)),
	).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect(networking.NewCFDomainValidator(k8sManager.GetClient()).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect((&korifiv1alpha1.CFRoute{}).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect(networking.NewCFRouteValidator(
		webhooks.NewDuplicateValidator(coordination.NewNameRegistry(k8sManager.GetClient(), networking.RouteEntityType)),
		rootNamespace,
		k8sManager.GetClient(),
	).SetupWebhookWithManager(k8sManager)).To(Succeed())

	err = shared.SetupIndexWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()

	Expect(k8sClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: rootNamespace,
		},
	})).To(Succeed())
})

var _ = AfterSuite(func() {
	cancel()
	Expect(testEnv.Stop()).To(Succeed())
})
