package integration_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/config"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/networking"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"k8s.io/client-go/kubernetes/scheme"
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

func TestNetworkingControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Networking Controllers Integration Suite")
}

var _ = BeforeSuite(func() {
	SetDefaultEventuallyTimeout(10 * time.Second)
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	var ctx context.Context
	ctx, cancel = context.WithCancel(context.TODO())

	// TODO: Add directory path for Contour CRDs
	testEnv = &envtest.Environment{
		// TODO: Reconcile with CRDInstallOptions
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "..", "dependencies"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	Expect(networkingv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(contourv1.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

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

	err = (&CFDomainReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&CFRouteReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
		Log:    ctrl.Log.WithName("controllers").WithName("CFRoute"),
		ControllerConfig: &config.ControllerConfig{
			KpackImageTag: "image/registry/tag",
			CFProcessDefaults: config.CFProcessDefaults{
				MemoryMB:           500,
				DefaultDiskQuotaMB: 512,
			},
			CFK8sControllerNamespace: "cf-k8s-controllers-system",
			WorkloadsTLSSecretName:   "cf-k8s-workloads-ingress-cert",
		},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	cancel()
	Expect(testEnv.Stop()).To(Succeed())
})
