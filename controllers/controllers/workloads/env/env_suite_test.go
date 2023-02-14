package env_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	servicebindingv1beta1 "github.com/servicebinding/service-binding-controller/apis/v1beta1"
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

var (
	cancel        context.CancelFunc
	testEnv       *envtest.Environment
	k8sClient     client.Client
	rootNamespace string
	cfOrg         *korifiv1alpha1.CFOrg
	cfSpace       *korifiv1alpha1.CFSpace
	ctx           context.Context
)

func TestEnvBuilders(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Env builders Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))

	mgrCtx, cancelFunc := context.WithCancel(context.TODO())
	cancel = cancelFunc

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "helm", "korifi", "controllers", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(servicebindingv1beta1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme.Scheme)).To(Succeed())

	//+kubebuilder:scaffold:scheme

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	Expect(err).NotTo(HaveOccurred())
	k8sClient = k8sManager.GetClient()

	err = shared.SetupIndexWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(mgrCtx)
		Expect(err).NotTo(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	cancel()
	Expect(testEnv.Stop()).To(Succeed())
})

var _ = BeforeEach(func() {
	ctx = context.Background()
	rootNamespace = testutils.PrefixedGUID("root-namespace")
	createNamespace(rootNamespace)

	cfOrg = &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testutils.PrefixedGUID("org"),
			Namespace: rootNamespace,
		},
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: testutils.PrefixedGUID("org"),
		},
	}
	createWithStatus(cfOrg, func(cfOrg *korifiv1alpha1.CFOrg) {
		cfOrg.Status.Conditions = []metav1.Condition{}
		cfOrg.Status.GUID = testutils.PrefixedGUID("org")
	})
	createNamespace(cfOrg.Status.GUID)

	cfSpace = &korifiv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testutils.PrefixedGUID("space"),
			Namespace: cfOrg.Status.GUID,
		},
		Spec: korifiv1alpha1.CFSpaceSpec{
			DisplayName: testutils.PrefixedGUID("space"),
		},
	}
	createWithStatus(cfSpace, func(cfSpace *korifiv1alpha1.CFSpace) {
		cfSpace.Status.Conditions = []metav1.Condition{}
		cfSpace.Status.GUID = testutils.PrefixedGUID("space")
	})
	createNamespace(cfSpace.Status.GUID)
})

func createNamespace(name string) *corev1.Namespace {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	return ns
}

func createWithStatus[T any, PT k8s.ObjectWithDeepCopy[T]](obj PT, setStatus func(PT)) PT {
	Expect(k8sClient.Create(ctx, obj)).To(Succeed())
	Expect(k8s.Patch(ctx, k8sClient, obj, func() {
		setStatus(obj)
	})).To(Succeed())
	return obj
}

func deleteAndWait[T client.Object](o T) {
	Expect(k8sClient.Delete(ctx, o)).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(o), o)).To(MatchError(ContainSubstring("not found")))
	}).Should(Succeed())
}

func clearSecretDataAndWait(s *corev1.Secret) {
	Expect(k8s.Patch(ctx, k8sClient, s, func() {
		s.Data = map[string][]byte{}
	})).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(s), s)).To(Succeed())
		g.Expect(s.Data).To(BeEmpty())
	}).Should(Succeed())
}

func patchCFAppAndWait(cfApp *korifiv1alpha1.CFApp, setFn func(a *korifiv1alpha1.CFApp), checkFn func(a *korifiv1alpha1.CFApp) bool) {
	Expect(k8s.Patch(ctx, k8sClient, cfApp, func() {
		setFn(cfApp)
	})).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
		g.Expect(checkFn(cfApp)).To(BeTrue())
	}).Should(Succeed())
}
