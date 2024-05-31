package docker_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/build/docker"
	buildfake "code.cloudfoundry.org/korifi/controllers/controllers/workloads/build/fake"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/oci"
	"code.cloudfoundry.org/korifi/tools/image"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes"
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
	stopClientCache   context.CancelFunc
	testEnv           *envtest.Environment
	adminClient       client.Client
	testNamespace     string
	containerRegistry *oci.Registry
)

func TestWorkloadsControllers(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Docker CFBuild Controllers Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))

	ctx = context.Background()

	containerRegistry = oci.NewContainerRegistry("user", "password")

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "..", "helm", "korifi", "controllers", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	_, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sManager := helpers.NewK8sManager(testEnv, filepath.Join("helm", "korifi", "controllers", "role.yaml"))
	Expect(shared.SetupIndexWithManager(k8sManager)).To(Succeed())

	adminClient, stopClientCache = helpers.NewCachedClient(testEnv.Config)

	k8sClient, err := k8sclient.NewForConfig(k8sManager.GetConfig())
	Expect(err).NotTo(HaveOccurred())

	err = docker.NewReconciler(
		k8sManager.GetClient(),
		new(buildfake.BuildCleaner),
		image.NewClient(k8sClient),
		k8sManager.GetScheme(),
		ctrl.Log.WithName("controllers").WithName("CFDockerBuild"),
	).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	stopManager = helpers.StartK8sManager(k8sManager)
})

var _ = BeforeEach(func() {
	testNamespace = uuid.NewString()
	Expect(adminClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	})).To(Succeed())
})

var _ = AfterSuite(func() {
	stopManager()
	stopClientCache()
	Expect(testEnv.Stop()).To(Succeed())
})
