package integration_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"code.cloudfoundry.org/korifi/controllers/apis/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	. "code.cloudfoundry.org/korifi/controllers/controllers/shared"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/fake"

	eiriniv1 "code.cloudfoundry.org/eirini-controller/pkg/apis/eirini/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	servicebindingv1beta1 "github.com/servicebinding/service-binding-controller/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	hncv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
	//+kubebuilder:scaffold:imports
)

var (
	cancel                  context.CancelFunc
	testEnv                 *envtest.Environment
	k8sClient               client.Client
	cfBuildReconciler       *CFBuildReconciler
	fakeImageProcessFetcher *fake.ImageProcessFetcher
)

const (
	packageRegistrySecretName = "test-package-registry-secret"
)

func TestWorkloadsControllers(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Workloads Controllers Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancelFunc := context.WithCancel(context.TODO())
	cancel = cancelFunc

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
			filepath.Join("fixtures", "vendor", "hierarchical-namespaces", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
		// TODO: Reconcile with CRDDirectoryPaths
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "..", "dependencies", "kpack-release-0.5.2.yaml"),
				filepath.Join("fixtures", "lrp-crd.yaml"),
			},
		},
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	Expect(v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	Expect(buildv1alpha2.AddToScheme(scheme.Scheme)).To(Succeed())
	// Add Eirini to Scheme
	Expect(eiriniv1.AddToScheme(scheme.Scheme)).To(Succeed())
	// Add hierarchical namespaces
	Expect(hncv1alpha2.AddToScheme(scheme.Scheme)).To(Succeed())

	Expect(servicebindingv1beta1.AddToScheme(scheme.Scheme)).To(Succeed())

	//+kubebuilder:scaffold:scheme

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
	Expect(err).NotTo(HaveOccurred())

	controllerConfig := &config.ControllerConfig{
		KpackImageTag:      "image/registry/tag",
		ClusterBuilderName: "cf-kpack-builder",
		CFProcessDefaults: config.CFProcessDefaults{
			MemoryMB:           500,
			DefaultDiskQuotaMB: 512,
		},
		KorifiControllerNamespace: "korifi-controllers-system",
		PackageRegistrySecretName: packageRegistrySecretName,
		WorkloadsTLSSecretName:    "korifi-workloads-ingress-cert",
	}

	err = (&CFAppReconciler{
		Client:           k8sManager.GetClient(),
		Scheme:           k8sManager.GetScheme(),
		Log:              ctrl.Log.WithName("controllers").WithName("CFApp"),
		ControllerConfig: controllerConfig,
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	registryAuthFetcherClient, err := k8sclient.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(registryAuthFetcherClient).NotTo(BeNil())
	cfBuildReconciler = &CFBuildReconciler{
		Client:              k8sManager.GetClient(),
		Scheme:              k8sManager.GetScheme(),
		Log:                 ctrl.Log.WithName("controllers").WithName("CFBuild"),
		ControllerConfig:    controllerConfig,
		RegistryAuthFetcher: NewRegistryAuthFetcher(registryAuthFetcherClient),
		EnvBuilder:          env.NewBuilder(k8sManager.GetClient()),
	}
	err = (cfBuildReconciler).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = (&CFProcessReconciler{
		Client:     k8sManager.GetClient(),
		Scheme:     k8sManager.GetScheme(),
		Log:        ctrl.Log.WithName("controllers").WithName("CFProcess"),
		EnvBuilder: env.NewBuilder(k8sManager.GetClient()),
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = (&CFPackageReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
		Log:    ctrl.Log.WithName("controllers").WithName("CFPackage"),
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = NewCFOrgReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		ctrl.Log.WithName("controllers").WithName("CFOrg"),
	).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = NewCFSpaceReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		ctrl.Log.WithName("controllers").WithName("CFSpace"),
		packageRegistrySecretName,
	).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	// Add new reconcilers here

	// Setup index for manager
	err = SetupIndexWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	cancel()
	Expect(testEnv.Stop()).To(Succeed())
})

var _ = BeforeEach(func() {
	fakeImageProcessFetcher = new(fake.ImageProcessFetcher)
	cfBuildReconciler.ImageProcessFetcher = fakeImageProcessFetcher.Spy
})

func createBuildWithDroplet(ctx context.Context, k8sClient client.Client, cfBuild *v1alpha1.CFBuild, droplet *v1alpha1.BuildDropletStatus) *v1alpha1.CFBuild {
	Expect(
		k8sClient.Create(ctx, cfBuild),
	).To(Succeed())
	patchedCFBuild := cfBuild.DeepCopy()
	patchedCFBuild.Status.Conditions = []metav1.Condition{}
	patchedCFBuild.Status.BuildDropletStatus = droplet
	Expect(
		k8sClient.Status().Patch(ctx, patchedCFBuild, client.MergeFrom(cfBuild)),
	).To(Succeed())
	return patchedCFBuild
}

func createNamespace(ctx context.Context, k8sClient client.Client, name string) *corev1.Namespace {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(
		k8sClient.Create(ctx, ns)).To(Succeed())
	return ns
}

func createNamespaceWithCleanup(ctx context.Context, k8sClient client.Client, name string) *corev1.Namespace {
	namespace := createNamespace(ctx, k8sClient, name)

	DeferCleanup(func() {
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	return namespace
}

func patchAppWithDroplet(ctx context.Context, k8sClient client.Client, appGUID, spaceGUID, buildGUID string) *v1alpha1.CFApp {
	baseCFApp := &v1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appGUID,
			Namespace: spaceGUID,
		},
	}
	patchedCFApp := baseCFApp.DeepCopy()
	patchedCFApp.Spec.CurrentDropletRef = corev1.LocalObjectReference{Name: buildGUID}
	Expect(k8sClient.Patch(ctx, patchedCFApp, client.MergeFrom(baseCFApp))).To(Succeed())
	return baseCFApp
}

func getMapKeyValue(m map[string]string, k string) string {
	if m == nil {
		return ""
	}
	if v, has := m[k]; has {
		return v
	}
	return ""
}
