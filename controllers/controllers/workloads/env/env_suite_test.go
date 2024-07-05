package env_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/helpers"

	"code.cloudfoundry.org/korifi/controllers/controllers/shared"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	servicebindingv1beta1 "github.com/servicebinding/runtime/apis/v1beta1"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	stopManager       context.CancelFunc
	stopClientCache   context.CancelFunc
	testEnv           *envtest.Environment
	adminClient       client.Client
	controllersClient client.Client
	rootNamespace     string
	cfOrg             *korifiv1alpha1.CFOrg
	cfSpace           *korifiv1alpha1.CFSpace
	ctx               context.Context
	cfApp             *korifiv1alpha1.CFApp
)

func TestEnvBuilders(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Env builders Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "helm", "korifi", "controllers", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	_, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(servicebindingv1beta1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sManager := helpers.NewK8sManager(testEnv, filepath.Join("helm", "korifi", "controllers", "role.yaml"))
	Expect(shared.SetupIndexWithManager(k8sManager)).To(Succeed())

	adminClient, stopClientCache = helpers.NewCachedClient(testEnv.Config)
	controllersClient = helpers.NewSyncClient(k8sManager.GetClient())

	stopManager = helpers.StartK8sManager(k8sManager)
})

var _ = AfterSuite(func() {
	stopClientCache()
	stopManager()
	Expect(testEnv.Stop()).To(Succeed())
})

var _ = BeforeEach(func() {
	ctx = context.Background()
	rootNamespace = uuid.NewString()
	createNamespace(rootNamespace)

	cfOrg = &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: rootNamespace,
		},
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: uuid.NewString(),
		},
	}
	helpers.EnsureCreate(controllersClient, cfOrg)
	orgNSName := uuid.NewString()
	helpers.EnsurePatch(controllersClient, cfOrg, func(cfOrg *korifiv1alpha1.CFOrg) {
		cfOrg.Status.GUID = orgNSName
	})
	createNamespace(cfOrg.Status.GUID)

	cfSpace = &korifiv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: cfOrg.Status.GUID,
		},
		Spec: korifiv1alpha1.CFSpaceSpec{
			DisplayName: uuid.NewString(),
		},
	}
	helpers.EnsureCreate(controllersClient, cfSpace)
	cfNSName := uuid.NewString()
	helpers.EnsurePatch(controllersClient, cfSpace, func(cfSpace *korifiv1alpha1.CFSpace) {
		cfSpace.Status.GUID = cfNSName
	})
	createNamespace(cfSpace.Status.GUID)

	cfApp = &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfSpace.Status.GUID,
			Name:      "app-guid",
		},
		Spec: korifiv1alpha1.CFAppSpec{
			EnvSecretName: "app-env-secret",
			DisplayName:   "app-display-name",
			DesiredState:  korifiv1alpha1.StoppedState,
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
			},
		},
	}
	helpers.EnsureCreate(controllersClient, cfApp)
	helpers.EnsurePatch(controllersClient, cfApp, func(app *korifiv1alpha1.CFApp) {
		app.Status = korifiv1alpha1.CFAppStatus{
			VCAPServicesSecretName:    "app-guid-vcap-services",
			VCAPApplicationSecretName: "app-guid-vcap-application",
		}
		meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.StatusConditionReady,
			Status:             metav1.ConditionTrue,
			Reason:             "testing",
			LastTransitionTime: metav1.Date(2023, 2, 15, 12, 0, 0, 0, time.FixedZone("", 0)),
		})
	})
})

func createNamespace(name string) *corev1.Namespace {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(adminClient.Create(ctx, ns)).To(Succeed())
	return ns
}
