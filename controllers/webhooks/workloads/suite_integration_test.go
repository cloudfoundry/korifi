package workloads_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/coordination"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/tests/helpers"

	"code.cloudfoundry.org/korifi/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/controllers/webhooks/version"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

var (
	stopManager     context.CancelFunc
	stopClientCache context.CancelFunc
	testEnv         *envtest.Environment
	adminClient     client.Client
)

const rootNamespace = "cf"

func TestWorkloadsWebhooks(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Workloads Validating Webhooks Integration Test Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "helm", "korifi", "controllers", "crds"),
		},
		ErrorIfCRDPathMissing: true,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "..", "helm", "korifi", "controllers", "manifests.yaml")},
		},
	}

	adminConfig, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(adminConfig).NotTo(BeNil())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(admissionv1beta1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(coordinationv1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(rbacv1.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sManager := helpers.NewK8sManager(testEnv, filepath.Join("helm", "korifi", "controllers", "role.yaml"))
	Expect(shared.SetupIndexWithManager(k8sManager)).To(Succeed())

	adminClient, stopClientCache = helpers.NewCachedClient(testEnv.Config)

	Expect((&korifiv1alpha1.CFApp{}).SetupWebhookWithManager(k8sManager)).To(Succeed())

	(&workloads.AppRevWebhook{}).SetupWebhookWithManager(k8sManager)

	appNameDuplicateValidator := webhooks.NewDuplicateValidator(coordination.NewNameRegistry(k8sManager.GetClient(), workloads.AppEntityType))
	Expect(workloads.NewCFAppValidator(appNameDuplicateValidator).SetupWebhookWithManager(k8sManager)).To(Succeed())

	orgNameDuplicateValidator := webhooks.NewDuplicateValidator(coordination.NewNameRegistry(k8sManager.GetClient(), workloads.CFOrgEntityType))
	orgPlacementValidator := webhooks.NewPlacementValidator(k8sManager.GetClient(), rootNamespace)
	Expect(workloads.NewCFOrgValidator(orgNameDuplicateValidator, orgPlacementValidator).SetupWebhookWithManager(k8sManager)).To(Succeed())

	spaceNameDuplicateValidator := webhooks.NewDuplicateValidator(coordination.NewNameRegistry(k8sManager.GetClient(), workloads.CFSpaceEntityType))
	spacePlacementValidator := webhooks.NewPlacementValidator(k8sManager.GetClient(), rootNamespace)
	Expect(workloads.NewCFSpaceValidator(spaceNameDuplicateValidator, spacePlacementValidator).SetupWebhookWithManager(k8sManager)).To(Succeed())

	Expect(workloads.NewCFTaskDefaulter(config.CFProcessDefaults{
		MemoryMB:    500,
		DiskQuotaMB: 512,
	}).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect(workloads.NewCFTaskValidator().SetupWebhookWithManager(k8sManager)).To(Succeed())
	version.NewVersionWebhook("some-version").SetupWebhookWithManager(k8sManager)
	finalizer.NewControllersFinalizerWebhook().SetupWebhookWithManager(k8sManager)

	stopManager = helpers.StartK8sManager(k8sManager)

	Expect(adminClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: rootNamespace,
		},
	})).To(Succeed())
})

var _ = AfterSuite(func() {
	stopClientCache()
	stopManager()
	Expect(testEnv.Stop()).To(Succeed())
})
