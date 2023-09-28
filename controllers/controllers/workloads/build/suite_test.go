package build_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/build"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/build/fake"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/controllers/coordination"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/controllers/webhooks/version"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	ctx               context.Context
	stopManager       context.CancelFunc
	stopClientCache   context.CancelFunc
	testEnv           *envtest.Environment
	adminClient       client.Client
	controllersClient client.Client
	testNamespace     string

	reconciledBuildsSync sync.Map
	buildCleanupsSync    sync.Map
)

func TestWorkloadsControllers(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)
	SetDefaultConsistentlyDuration(5 * time.Second)
	SetDefaultConsistentlyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Build Controller Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))

	ctx = context.Background()

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "helm", "korifi", "controllers", "crds"),
		},
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "..", "..", "helm", "korifi", "controllers", "manifests.yaml")},
		},
		ErrorIfCRDPathMissing: true,
	}

	_, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sManager := helpers.NewK8sManager(testEnv, filepath.Join("helm", "korifi", "controllers", "role.yaml"))
	Expect(shared.SetupIndexWithManager(k8sManager)).To(Succeed())

	Expect((&korifiv1alpha1.CFApp{}).SetupWebhookWithManager(k8sManager)).To(Succeed())
	finalizer.NewControllersFinalizerWebhook().SetupWebhookWithManager(k8sManager)
	version.NewVersionWebhook("some-version").SetupWebhookWithManager(k8sManager)
	Expect(workloads.NewCFAppValidator(
		webhooks.NewDuplicateValidator(coordination.NewNameRegistry(k8sManager.GetClient(), workloads.AppEntityType)),
	).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect((&korifiv1alpha1.CFPackage{}).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect((&korifiv1alpha1.CFBuild{}).SetupWebhookWithManager(k8sManager)).To(Succeed())

	controllersClient = k8sManager.GetClient()
	adminClient, stopClientCache = helpers.NewCachedClient(testEnv.Config)

	testNamespace = testutils.PrefixedGUID("test-namespace")

	delegateReconciler := new(fake.BuildReconciler)
	delegateReconciler.SetupWithManagerStub = func(mgr ctrl.Manager) *builder.Builder {
		return ctrl.NewControllerManagedBy(mgr).
			For(&korifiv1alpha1.CFBuild{})
	}
	reconciledBuildsSync = sync.Map{}
	delegateReconciler.ReconcileBuildStub = func(ctx context.Context, cfBuild *korifiv1alpha1.CFBuild, _ *korifiv1alpha1.CFApp, _ *korifiv1alpha1.CFPackage) (reconcile.Result, error) {
		log := logr.FromContextOrDiscard(ctx)

		currentValue, ok := reconciledBuildsSync.Load(cfBuild.Name)
		currentCount := 0
		if ok {
			currentCount = currentValue.(int)
		}
		reconciledBuildsSync.Store(cfBuild.Name, currentCount+1)

		log.Info("set delegateInvokedCondition", "cfBuild", cfBuild.Name, "namespace", cfBuild.Namespace)
		meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
			Type:               "delegateInvokedCondition",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cfBuild.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             "delegateInvoked",
			Message:            "this status condition is a marker to signal that the delegate reconciler has been invoked",
		})
		return reconcile.Result{}, nil
	}

	buildCleanupsSync = sync.Map{}
	buildCleaner := new(fake.BuildCleaner)
	buildCleaner.CleanStub = func(_ context.Context, nsname types.NamespacedName) error {
		currentValue, ok := buildCleanupsSync.Load(nsname)
		currentCount := 0
		if ok {
			currentCount = currentValue.(int)
		}
		buildCleanupsSync.Store(nsname, currentCount+1)
		return nil
	}

	Expect(k8s.NewPatchingReconciler[korifiv1alpha1.CFBuild, *korifiv1alpha1.CFBuild](
		ctrl.Log.WithName("controllers").WithName("CFBuild"),
		k8sManager.GetClient(),
		build.NewCFBuildReconciler(
			ctrl.Log.WithName("controllers").WithName("CFBuild"),
			k8sManager.GetClient(),
			scheme.Scheme,
			buildCleaner,
			delegateReconciler,
		),
	).SetupWithManager(k8sManager)).To(Succeed())

	stopManager = helpers.StartK8sManager(k8sManager)

	createNamespace(testNamespace)
})

var _ = AfterSuite(func() {
	stopManager()
	stopClientCache()
	Expect(testEnv.Stop()).To(Succeed())
})

func createNamespace(name string) *corev1.Namespace {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(
		adminClient.Create(ctx, ns)).To(Succeed())
	return ns
}
