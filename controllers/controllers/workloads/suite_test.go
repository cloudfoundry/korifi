package workloads_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	buildfake "code.cloudfoundry.org/korifi/controllers/controllers/workloads/build/fake"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/fake"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/labels"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/controllers/coordination"
	controllerfake "code.cloudfoundry.org/korifi/controllers/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/controllers/webhooks/networking"
	"code.cloudfoundry.org/korifi/controllers/webhooks/services"
	"code.cloudfoundry.org/korifi/controllers/webhooks/version"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/oci"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/image"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	servicebindingv1beta1 "github.com/servicebinding/runtime/apis/v1beta1"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	admission "k8s.io/pod-security-admission/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	ctx                  context.Context
	stopManager          context.CancelFunc
	stopClientCache      context.CancelFunc
	testEnv              *envtest.Environment
	adminClient          client.Client
	controllersClient    client.Client
	cfRootNamespace      string
	cfOrg                *korifiv1alpha1.CFOrg
	imageRegistrySecret1 *corev1.Secret
	imageRegistrySecret2 *corev1.Secret
	imageDeleter         *fake.ImageDeleter
	packageCleaner       *fake.PackageCleaner
	eventRecorder        *controllerfake.EventRecorder
	buildCleaner         *buildfake.BuildCleaner
	imageClient          image.Client
	containerRegistry    *oci.Registry
)

const (
	defaultMemoryMB    = 128
	defaultDiskQuotaMB = 256
	defaultTimeout     = 60

	packageRegistrySecretName = "test-package-registry-secret"
	otherRegistrySecretName   = "some-other-registry-secret"
)

func TestWorkloadsControllers(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Workloads Controllers Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))

	ctx = context.Background()

	containerRegistry = oci.NewContainerRegistry("user", "password")

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "helm", "korifi", "controllers", "crds"),
		},
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "..", "helm", "korifi", "controllers", "manifests.yaml")},
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

	controllersClient = k8sManager.GetClient()
	adminClient, stopClientCache = helpers.NewCachedClient(testEnv.Config)

	cfRootNamespace = testutils.PrefixedGUID("root-namespace")

	controllerConfig := &config.ControllerConfig{
		CFProcessDefaults: config.CFProcessDefaults{
			MemoryMB:    500,
			DiskQuotaMB: 512,
		},
		CFRootNamespace:                  cfRootNamespace,
		ContainerRegistrySecretNames:     []string{packageRegistrySecretName, otherRegistrySecretName},
		WorkloadsTLSSecretName:           "korifi-workloads-ingress-cert",
		WorkloadsTLSSecretNamespace:      "korifi-controllers-system",
		SpaceFinalizerAppDeletionTimeout: tools.PtrTo(int64(2)),
	}

	k8sClient, err := k8sclient.NewForConfig(k8sManager.GetConfig())
	Expect(err).NotTo(HaveOccurred())
	imageClient = image.NewClient(k8sClient)

	eventRecorder = new(controllerfake.EventRecorder)

	err = (NewCFAppReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		ctrl.Log.WithName("controllers").WithName("CFApp"),
		env.NewVCAPServicesEnvValueBuilder(k8sManager.GetClient()),
		env.NewVCAPApplicationEnvValueBuilder(k8sManager.GetClient(), nil),
	)).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	buildCleaner = new(buildfake.BuildCleaner)
	cfBuildpackBuildReconciler := NewCFBuildpackBuildReconciler(
		k8sManager.GetClient(),
		buildCleaner,
		k8sManager.GetScheme(),
		ctrl.Log.WithName("controllers").WithName("CFBuildpackBuild"),
		controllerConfig,
		env.NewWorkloadEnvBuilder(k8sManager.GetClient()),
	)
	err = (cfBuildpackBuildReconciler).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	cfDockerBuildReconciler := NewCFDockerBuildReconciler(
		k8sManager.GetClient(),
		buildCleaner,
		imageClient,
		k8sManager.GetScheme(),
		ctrl.Log.WithName("controllers").WithName("CFDockerBuild"),
	)
	err = (cfDockerBuildReconciler).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = (NewCFProcessReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		ctrl.Log.WithName("controllers").WithName("CFProcess"),
		controllerConfig,
		env.NewWorkloadEnvBuilder(k8sManager.GetClient()),
	)).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	imageDeleter = new(fake.ImageDeleter)
	packageCleaner = new(fake.PackageCleaner)
	err = (NewCFPackageReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		ctrl.Log.WithName("controllers").WithName("CFPackage"),
		imageDeleter,
		packageCleaner,
		[]string{"package-repo-secret-name"},
	)).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	labelCompiler := labels.NewCompiler().Defaults(map[string]string{
		admission.EnforceLevelLabel: string(admission.LevelRestricted),
		admission.AuditLevelLabel:   string(admission.LevelRestricted),
	})

	err = NewCFOrgReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		ctrl.Log.WithName("controllers").WithName("CFOrg"),
		controllerConfig.ContainerRegistrySecretNames,
		labelCompiler,
	).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = NewCFSpaceReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		ctrl.Log.WithName("controllers").WithName("CFSpace"),
		controllerConfig.ContainerRegistrySecretNames,
		controllerConfig.CFRootNamespace,
		*controllerConfig.SpaceFinalizerAppDeletionTimeout,
		labelCompiler,
	).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = NewCFTaskReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		eventRecorder,
		ctrl.Log.WithName("controllers").WithName("CFTask"),
		env.NewWorkloadEnvBuilder(k8sManager.GetClient()),
		2*time.Second,
	).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	finalizer.NewControllersFinalizerWebhook().SetupWebhookWithManager(k8sManager)
	version.NewVersionWebhook("some-version").SetupWebhookWithManager(k8sManager)
	Expect((&korifiv1alpha1.CFApp{}).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect(workloads.NewCFAppValidator(
		webhooks.NewDuplicateValidator(coordination.NewNameRegistry(k8sManager.GetClient(), workloads.AppEntityType)),
	).SetupWebhookWithManager(k8sManager)).To(Succeed())
	(&workloads.AppRevWebhook{}).SetupWebhookWithManager(k8sManager)

	orgNameDuplicateValidator := webhooks.NewDuplicateValidator(coordination.NewNameRegistry(k8sManager.GetClient(), workloads.CFOrgEntityType))
	orgPlacementValidator := webhooks.NewPlacementValidator(k8sManager.GetClient(), cfRootNamespace)
	Expect(workloads.NewCFOrgValidator(orgNameDuplicateValidator, orgPlacementValidator).SetupWebhookWithManager(k8sManager)).To(Succeed())

	spaceNameDuplicateValidator := webhooks.NewDuplicateValidator(coordination.NewNameRegistry(k8sManager.GetClient(), workloads.CFSpaceEntityType))
	spacePlacementValidator := webhooks.NewPlacementValidator(k8sManager.GetClient(), cfRootNamespace)
	Expect(workloads.NewCFSpaceValidator(spaceNameDuplicateValidator, spacePlacementValidator).SetupWebhookWithManager(k8sManager)).To(Succeed())

	Expect(networking.NewCFDomainValidator(k8sManager.GetClient()).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect(services.NewCFServiceInstanceValidator(
		webhooks.NewDuplicateValidator(coordination.NewNameRegistry(k8sManager.GetClient(), services.ServiceInstanceEntityType)),
	).SetupWebhookWithManager(k8sManager)).To(Succeed())

	Expect((&korifiv1alpha1.CFPackage{}).SetupWebhookWithManager(k8sManager)).To(Succeed())

	Expect(workloads.NewCFTaskValidator().SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect(workloads.NewCFTaskDefaulter(config.CFProcessDefaults{
		MemoryMB:    128,
		DiskQuotaMB: 256,
	}).SetupWebhookWithManager(k8sManager)).To(Succeed())

	Expect(korifiv1alpha1.NewCFProcessDefaulter(defaultMemoryMB, defaultDiskQuotaMB, defaultTimeout).
		SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect((&korifiv1alpha1.CFBuild{}).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect((&korifiv1alpha1.CFRoute{}).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect(networking.NewCFRouteValidator(
		webhooks.NewDuplicateValidator(coordination.NewNameRegistry(k8sManager.GetClient(), networking.RouteEntityType)),
		cfRootNamespace,
		k8sManager.GetClient(),
	).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect(services.NewCFServiceBindingValidator(
		webhooks.NewDuplicateValidator(coordination.NewNameRegistry(k8sManager.GetClient(), services.ServiceBindingEntityType)),
	).SetupWebhookWithManager(k8sManager)).To(Succeed())

	stopManager = helpers.StartK8sManager(k8sManager)

	createNamespace(cfRootNamespace)
	imageRegistrySecret1 = createImageRegistrySecret(ctx, adminClient, packageRegistrySecretName, cfRootNamespace)
	imageRegistrySecret2 = createImageRegistrySecret(ctx, adminClient, otherRegistrySecretName, cfRootNamespace)

	cfOrg = createOrg(cfRootNamespace)
})

var _ = AfterSuite(func() {
	stopManager()
	stopClientCache()
	Expect(testEnv.Stop()).To(Succeed())
})

func createBuildWithDroplet(ctx context.Context, k8sClient client.Client, cfBuild *korifiv1alpha1.CFBuild, droplet *korifiv1alpha1.BuildDropletStatus) *korifiv1alpha1.CFBuild {
	Expect(
		k8sClient.Create(ctx, cfBuild),
	).To(Succeed())
	patchedCFBuild := cfBuild.DeepCopy()
	patchedCFBuild.Status.Droplet = droplet
	Expect(
		k8sClient.Status().Patch(ctx, patchedCFBuild, client.MergeFrom(cfBuild)),
	).To(Succeed())
	return patchedCFBuild
}

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

func createImageRegistrySecret(ctx context.Context, k8sClient client.Client, name string, namespace string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"kapp.k14s.io/foo": "bar",
				"meta.helm.sh/baz": "foo",
				"bar":              "baz",
			},
		},
		StringData: map[string]string{
			"foo": "bar",
		},
		Type: "Docker",
	}
	Expect(k8sClient.Create(ctx, secret)).To(Succeed())
	return secret
}

func createClusterRole(ctx context.Context, k8sClient client.Client, name string, rules []rbacv1.PolicyRule) *rbacv1.ClusterRole {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Rules: rules,
	}
	Expect(k8sClient.Create(ctx, role)).To(Succeed())
	return role
}

func createRoleBinding(ctx context.Context, k8sClient client.Client, roleBindingName, subjectName, roleReference, namespace string, annotations map[string]string) *rbacv1.RoleBinding {
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:        roleBindingName,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Subjects: []rbacv1.Subject{{
			Kind: rbacv1.UserKind,
			Name: subjectName,
		}},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: roleReference,
		},
	}
	Expect(k8sClient.Create(ctx, roleBinding)).To(Succeed())
	return roleBinding
}

func createServiceAccount(ctx context.Context, k8sclient client.Client, serviceAccountName, namespace string, annotations map[string]string) *corev1.ServiceAccount {
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
			{Name: otherRegistrySecretName},
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: serviceAccountName + "-dockercfg-someguid"},
			{Name: packageRegistrySecretName},
			{Name: otherRegistrySecretName},
		},
	}
	Expect(adminClient.Create(ctx, serviceAccount)).To(Succeed())
	return serviceAccount
}

func patchAppWithDroplet(ctx context.Context, k8sClient client.Client, appGUID, spaceGUID, buildGUID string) *korifiv1alpha1.CFApp {
	cfApp := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appGUID,
			Namespace: spaceGUID,
		},
	}
	Expect(k8s.Patch(ctx, k8sClient, cfApp, func() {
		cfApp.Spec.CurrentDropletRef = corev1.LocalObjectReference{Name: buildGUID}
	})).To(Succeed())
	return cfApp
}

func createOrg(rootNamespace string) *korifiv1alpha1.CFOrg {
	org := &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testutils.PrefixedGUID("org"),
			Namespace: rootNamespace,
		},
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: testutils.PrefixedGUID("org"),
		},
	}
	Expect(adminClient.Create(ctx, org)).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(org), org)).To(Succeed())
		g.Expect(meta.IsStatusConditionTrue(org.Status.Conditions, shared.StatusConditionReady)).To(BeTrue())
	}).Should(Succeed())
	return org
}

func createSpace(org *korifiv1alpha1.CFOrg) *korifiv1alpha1.CFSpace {
	cfSpace := &korifiv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testutils.PrefixedGUID("space"),
			Namespace: org.Status.GUID,
		},
		Spec: korifiv1alpha1.CFSpaceSpec{
			DisplayName: testutils.PrefixedGUID("space"),
		},
	}
	Expect(adminClient.Create(ctx, cfSpace)).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfSpace), cfSpace)).To(Succeed())
		g.Expect(meta.IsStatusConditionTrue(cfSpace.Status.Conditions, shared.StatusConditionReady)).To(BeTrue())
	}).Should(Succeed())
	return cfSpace
}
