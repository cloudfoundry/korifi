package finalizer_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/coordination"
	"code.cloudfoundry.org/korifi/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/controllers/webhooks/networking/domains"
	"code.cloudfoundry.org/korifi/controllers/webhooks/networking/routes"
	"code.cloudfoundry.org/korifi/controllers/webhooks/services/instances"
	"code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	"code.cloudfoundry.org/korifi/controllers/webhooks/version"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/apps"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/orgs"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/packages"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/spaces"
	"code.cloudfoundry.org/korifi/tests/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	coordinationv1 "k8s.io/api/coordination/v1"
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
	stopManager     context.CancelFunc
	stopClientCache context.CancelFunc
	testEnv         *envtest.Environment
	adminClient     client.Client
)

const (
	rootNamespace     = "cf"
	defaultDomainName = "default-domain"
)

func TestWorkloadsWebhooks(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Finalizer Webhook Integration Test Suite")
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

	_, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(admissionv1beta1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(coordinationv1.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sManager := helpers.NewK8sManager(testEnv, filepath.Join("helm", "korifi", "controllers", "role.yaml"))
	Expect(shared.SetupIndexWithManager(k8sManager)).To(Succeed())

	adminClient, stopClientCache = helpers.NewCachedClient(testEnv.Config)

	finalizer.NewControllersFinalizerWebhook().SetupWebhookWithManager(k8sManager)

	version.NewVersionWebhook("some-version").SetupWebhookWithManager(k8sManager)
	Expect((&korifiv1alpha1.CFApp{}).SetupWebhookWithManager(k8sManager)).To(Succeed())

	uncachedClient := helpers.NewUncachedClient(k8sManager.GetConfig())
	Expect(apps.NewValidator(
		validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, apps.AppEntityType)),
	).SetupWebhookWithManager(k8sManager)).To(Succeed())

	orgNameDuplicateValidator := validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, orgs.CFOrgEntityType))
	orgPlacementValidator := validation.NewPlacementValidator(uncachedClient, rootNamespace)
	Expect(orgs.NewValidator(orgNameDuplicateValidator, orgPlacementValidator).SetupWebhookWithManager(k8sManager)).To(Succeed())

	spaceNameDuplicateValidator := validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, spaces.CFSpaceEntityType))
	spacePlacementValidator := validation.NewPlacementValidator(uncachedClient, rootNamespace)
	Expect(spaces.NewValidator(spaceNameDuplicateValidator, spacePlacementValidator).SetupWebhookWithManager(k8sManager)).To(Succeed())

	Expect(domains.NewValidator(uncachedClient).SetupWebhookWithManager(k8sManager)).To(Succeed())

	Expect((&korifiv1alpha1.CFPackage{}).SetupWebhookWithManager(k8sManager)).To(Succeed())

	Expect((&korifiv1alpha1.CFRoute{}).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect(routes.NewValidator(
		validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, routes.RouteEntityType)),
		rootNamespace,
		uncachedClient,
	).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect(packages.NewValidator().SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect(instances.NewValidator(validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, instances.ServiceInstanceEntityType))).SetupWebhookWithManager(k8sManager)).To(Succeed())

	stopManager = helpers.StartK8sManager(k8sManager)

	ctx := context.Background()
	Expect(adminClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: rootNamespace,
		},
	})).To(Succeed())

	Expect(adminClient.Create(ctx, &korifiv1alpha1.CFDomain{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: rootNamespace,
			Name:      defaultDomainName,
		},
		Spec: korifiv1alpha1.CFDomainSpec{
			Name: "my.domain",
		},
	})).To(Succeed())
})

var _ = AfterSuite(func() {
	stopClientCache()
	stopManager()
	Expect(testEnv.Stop()).To(Succeed())
})
