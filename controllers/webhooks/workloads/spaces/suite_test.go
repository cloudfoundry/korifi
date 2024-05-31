package spaces_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/coordination"
	"code.cloudfoundry.org/korifi/tests/helpers"

	"code.cloudfoundry.org/korifi/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	"code.cloudfoundry.org/korifi/controllers/webhooks/version"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/orgs"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/spaces"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

var (
	stopManager        context.CancelFunc
	stopClientCache    context.CancelFunc
	testEnv            *envtest.Environment
	adminClient        client.Client
	adminNonSyncClient client.Client

	ctx           context.Context
	rootNamespace string
)

func TestWorkloadsWebhooks(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "CFSpace Webhooks Integration Test Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx = context.Background()

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "helm", "korifi", "controllers", "crds"),
		},
		ErrorIfCRDPathMissing: true,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "..", "..", "helm", "korifi", "controllers", "manifests.yaml")},
		},
	}

	adminConfig, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(adminConfig).NotTo(BeNil())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sManager := helpers.NewK8sManager(testEnv, filepath.Join("helm", "korifi", "controllers", "role.yaml"))
	Expect(shared.SetupIndexWithManager(k8sManager)).To(Succeed())

	adminNonSyncClient, err = client.New(testEnv.Config, client.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	adminClient, stopClientCache = helpers.NewCachedClient(testEnv.Config)

	rootNamespace = uuid.NewString()
	Expect(adminClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: rootNamespace,
		},
	})).To(Succeed())

	uncachedClient := helpers.NewUncachedClient(k8sManager.GetConfig())

	version.NewVersionWebhook("some-version").SetupWebhookWithManager(k8sManager)
	finalizer.NewControllersFinalizerWebhook().SetupWebhookWithManager(k8sManager)

	orgNameDuplicateValidator := validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, orgs.CFOrgEntityType))
	orgPlacementValidator := validation.NewPlacementValidator(uncachedClient, rootNamespace)
	Expect(orgs.NewValidator(orgNameDuplicateValidator, orgPlacementValidator).SetupWebhookWithManager(k8sManager)).To(Succeed())

	spaceNameDuplicateValidator := validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, spaces.CFSpaceEntityType))
	spacePlacementValidator := validation.NewPlacementValidator(uncachedClient, rootNamespace)
	Expect(spaces.NewValidator(spaceNameDuplicateValidator, spacePlacementValidator).SetupWebhookWithManager(k8sManager)).To(Succeed())

	stopManager = helpers.StartK8sManager(k8sManager)
})

var _ = AfterSuite(func() {
	stopClientCache()
	stopManager()
	Expect(testEnv.Stop()).To(Succeed())
})
