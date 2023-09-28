package k8sns_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/k8sns"
	"code.cloudfoundry.org/korifi/tests/helpers"

	"github.com/google/uuid"
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

var (
	ctx               context.Context
	stopManager       context.CancelFunc
	testEnv           *envtest.Environment
	adminClient       client.Client
	controllersClient client.Client
	rootNamespace     string
)

func TestK8sNS(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "K8S NS Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))

	ctx = context.Background()

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "helm", "korifi", "controllers", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	_, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sManager := helpers.NewK8sManager(testEnv, filepath.Join("helm", "korifi", "controllers", "role.yaml"))
	Expect(shared.SetupIndexWithManager(k8sManager)).To(Succeed())

	adminClient, err = client.New(testEnv.Config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	adminClient = helpers.NewSyncClient(adminClient)

	controllersClient, err = client.New(helpers.SetupTestEnvUser(testEnv, filepath.Join("helm", "korifi", "controllers", "role.yaml")), client.Options{})
	Expect(err).NotTo(HaveOccurred())
	controllersClient = helpers.NewSyncClient(controllersClient)

	stopManager = helpers.StartK8sManager(k8sManager)
})

var _ = BeforeEach(func() {
	rootNamespace = uuid.NewString()
})

var _ = AfterSuite(func() {
	stopManager()
	Expect(testEnv.Stop()).To(Succeed())
})

type mockFinalizer[T any, NS k8sns.NamespaceObject[T]] struct {
	finalizedObjects []NS
	result           ctrl.Result
	finalizeErr      error
}

func (f *mockFinalizer[T, NS]) Finalize(ctx context.Context, obj NS) (ctrl.Result, error) {
	f.finalizedObjects = append(f.finalizedObjects, obj)
	return f.result, f.finalizeErr
}

func createNamespace(name string) *corev1.Namespace {
	GinkgoHelper()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(
		adminClient.Create(ctx, ns)).To(Succeed())
	return ns
}

func getNamespace(nsName string) *corev1.Namespace {
	GinkgoHelper()

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}
	Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace)).To(Succeed())

	return namespace
}
