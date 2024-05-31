package spaces_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/labels"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/spaces"
	"code.cloudfoundry.org/korifi/tests/helpers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	admission "k8s.io/pod-security-admission/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	ctx             context.Context
	stopManager     context.CancelFunc
	stopClientCache context.CancelFunc
	testEnv         *envtest.Environment
	adminClient     client.Client
	cfRootNamespace string
	testNamespace   string
)

const (
	packageRegistrySecretName = "test-package-registry-secret"
)

func TestWorkloadsControllers(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "CFSpace Controller Integration Suite")
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

	adminClient, stopClientCache = helpers.NewCachedClient(testEnv.Config)

	labelCompiler := labels.NewCompiler().Defaults(map[string]string{
		admission.EnforceLevelLabel: string(admission.LevelRestricted),
		admission.AuditLevelLabel:   string(admission.LevelRestricted),
	})

	cfRootNamespace = uuid.NewString()
	Expect(adminClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfRootNamespace,
		},
	})).To(Succeed())

	err = spaces.NewReconciler(
		k8sManager.GetClient(),
		ctrl.Log.WithName("controllers").WithName("CFSpace"),
		[]string{packageRegistrySecretName},
		cfRootNamespace,
		int64(2),
		labelCompiler,
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

	Expect(adminClient.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      packageRegistrySecretName,
			Namespace: testNamespace,
		},
	})).To(Succeed())
})

var _ = AfterSuite(func() {
	stopManager()
	stopClientCache()
	Expect(testEnv.Stop()).To(Succeed())
})

func createServiceAccount(ctx context.Context, serviceAccountName, namespace string, annotations map[string]string) *corev1.ServiceAccount {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        serviceAccountName,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Secrets: []corev1.ObjectReference{
			{Name: serviceAccountName + "-token-someguid"},
			{Name: serviceAccountName + "-dockercfg-someguid"},
			{Name: packageRegistrySecretName},
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: serviceAccountName + "-dockercfg-someguid"},
			{Name: packageRegistrySecretName},
		},
	}
	Expect(adminClient.Create(ctx, serviceAccount)).To(Succeed())
	return serviceAccount
}
