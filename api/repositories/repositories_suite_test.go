package repositories_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/authorization/testhelpers"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	timeCheckThreshold = 3 * time.Second
)

func TestRepositories(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Repositories Suite")
}

var (
	ctx                   context.Context
	testEnv               *envtest.Environment
	k8sClient             client.WithWatch
	namespaceRetriever    repositories.NamespaceRetriever
	userClientFactory     authorization.UserK8sClientFactory
	userName              string
	authInfo              authorization.Info
	rootNamespace         string
	builderName           string
	runnerName            string
	idProvider            authorization.IdentityProvider
	nsPerms               *authorization.NamespacePermissions
	adminRole             *rbacv1.ClusterRole
	spaceDeveloperRole    *rbacv1.ClusterRole
	spaceManagerRole      *rbacv1.ClusterRole
	orgManagerRole        *rbacv1.ClusterRole
	orgUserRole           *rbacv1.ClusterRole
	spaceAuditorRole      *rbacv1.ClusterRole
	rootNamespaceUserRole *rbacv1.ClusterRole
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "helm", "korifi", "controllers", "crds"),
			filepath.Join("..", "..", "tests", "vendor", "kpack"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	_, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	err = korifiv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = buildv1alpha2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.NewWithWatch(testEnv.Config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	dynamicClient, err := dynamic.NewForConfig(testEnv.Config)
	Expect(err).NotTo(HaveOccurred())
	Expect(dynamicClient).NotTo(BeNil())
	namespaceRetriever = repositories.NewNamespaceRetriever(dynamicClient)
	Expect(namespaceRetriever).NotTo(BeNil())

	adminRole = createClusterRole(context.Background(), "cf_admin")
	orgManagerRole = createClusterRole(context.Background(), "cf_org_manager")
	orgUserRole = createClusterRole(context.Background(), "cf_org_user")
	spaceDeveloperRole = createClusterRole(context.Background(), "cf_space_developer")
	spaceManagerRole = createClusterRole(context.Background(), "cf_space_manager")
	spaceAuditorRole = createClusterRole(context.Background(), "cf_space_auditor")
	rootNamespaceUserRole = createClusterRole(context.Background(), "cf_root_namespace_user")
})

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).To(Succeed())
})

var _ = BeforeEach(func() {
	ctx = context.Background()
	userName = uuid.NewString()
	cert, key := testhelpers.ObtainClientCert(testEnv, userName)
	authInfo.CertData = testhelpers.JoinCertAndKey(cert, key)
	rootNamespace = prefixedGUID("root-ns")
	builderName = "kpack-image-builder"
	runnerName = "statefulset-runner"
	tokenInspector := authorization.NewTokenReviewer(k8sClient)
	certInspector := authorization.NewCertInspector(testEnv.Config)
	baseIDProvider := authorization.NewCertTokenIdentityProvider(tokenInspector, certInspector)
	idProvider = authorization.NewCachingIdentityProvider(baseIDProvider, cache.NewExpiring())
	nsPerms = authorization.NewNamespacePermissions(k8sClient, idProvider)

	httpClient, err := rest.HTTPClientFor(testEnv.Config)
	Expect(err).NotTo(HaveOccurred())
	mapper, err := apiutil.NewDynamicRESTMapper(testEnv.Config, httpClient)
	Expect(err).NotTo(HaveOccurred())
	userClientFactory = authorization.NewUnprivilegedClientFactory(testEnv.Config, mapper, k8s.NewDefaultBackoff())

	Expect(k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}})).To(Succeed())
	createRoleBinding(context.Background(), userName, rootNamespaceUserRole.Name, rootNamespace)
})

var _ = AfterEach(func() {
	Expect(k8sClient.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}})).To(Succeed())
})

func createOrgWithCleanup(ctx context.Context, displayName string) *korifiv1alpha1.CFOrg {
	guid := uuid.NewString()
	cfOrg := &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: rootNamespace,
		},
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: displayName,
		},
	}
	Expect(k8sClient.Create(ctx, cfOrg)).To(Succeed())

	meta.SetStatusCondition(&(cfOrg.Status.Conditions), metav1.Condition{
		Type:    korifiv1alpha1.StatusConditionReady,
		Status:  metav1.ConditionTrue,
		Reason:  "cus",
		Message: "cus",
	})
	Expect(k8sClient.Status().Update(ctx, cfOrg)).To(Succeed())

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfOrg.Name,
			Labels: map[string]string{
				korifiv1alpha1.OrgNameKey: cfOrg.Spec.DisplayName,
			},
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

	DeferCleanup(func() {
		_ = k8sClient.Delete(ctx, cfOrg)
		_ = k8sClient.Delete(ctx, namespace)
	})

	return cfOrg
}

func createSpaceWithCleanup(ctx context.Context, orgGUID, name string) *korifiv1alpha1.CFSpace {
	guid := uuid.NewString()
	cfSpace := &korifiv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: orgGUID,
		},
		Spec: korifiv1alpha1.CFSpaceSpec{
			DisplayName: name,
		},
	}
	Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())

	cfSpace.Status.GUID = cfSpace.Name
	meta.SetStatusCondition(&(cfSpace.Status.Conditions), metav1.Condition{
		Type:    korifiv1alpha1.StatusConditionReady,
		Status:  metav1.ConditionTrue,
		Reason:  "cus",
		Message: "cus",
	})
	Expect(k8sClient.Status().Update(ctx, cfSpace)).To(Succeed())

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfSpace.Name,
			Labels: map[string]string{
				korifiv1alpha1.SpaceNameKey: cfSpace.Spec.DisplayName,
			},
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

	DeferCleanup(func() {
		_ = k8sClient.Delete(ctx, cfSpace)
		_ = k8sClient.Delete(ctx, namespace)
	})

	return cfSpace
}

func createClusterRole(ctx context.Context, filename string) *rbacv1.ClusterRole {
	filepath := filepath.Join("..", "..", "helm", "korifi", "controllers", "cf_roles", filename+".yaml")
	content, err := os.ReadFile(filepath)
	Expect(err).NotTo(HaveOccurred())

	decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDecoder()
	clusterRole := &rbacv1.ClusterRole{}
	err = runtime.DecodeInto(decoder, content, clusterRole)
	Expect(err).NotTo(HaveOccurred())

	clusterRole.Name = "cf-" + clusterRole.Name
	Expect(k8sClient.Create(ctx, clusterRole)).To(Succeed())

	return clusterRole
}

func createRoleBinding(ctx context.Context, userName, roleName, namespace string, labels ...string) rbacv1.RoleBinding {
	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: namespace,
			Labels:    map[string]string{},
		},
		Subjects: []rbacv1.Subject{{
			Kind: rbacv1.UserKind,
			Name: userName,
		}},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: roleName,
		},
	}

	for i := 0; i < len(labels); i += 2 {
		roleBinding.Labels[labels[i]] = labels[i+1]
	}

	Expect(k8sClient.Create(ctx, &roleBinding)).To(Succeed())
	return roleBinding
}

func prefixedGUID(prefix string) string {
	return prefix + "-" + uuid.NewString()[:8]
}

func createAppCR(ctx context.Context, k8sClient client.Client, appName, appGUID, spaceGUID, desiredState string) *korifiv1alpha1.CFApp {
	GinkgoHelper()

	toReturn := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appGUID,
			Namespace: spaceGUID,
		},
		Spec: korifiv1alpha1.CFAppSpec{
			DisplayName:  appName,
			DesiredState: korifiv1alpha1.AppState(desiredState),
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
				Data: korifiv1alpha1.LifecycleData{
					Buildpacks: []string{},
					Stack:      "",
				},
			},
		},
	}
	Expect(
		k8sClient.Create(ctx, toReturn),
	).To(Succeed())
	return toReturn
}

func createBuild(ctx context.Context, k8sClient client.Client, namespace, buildGUID, packageGUID, appGUID string) *korifiv1alpha1.CFBuild {
	const (
		stagingMemory = 1024
		stagingDisk   = 2048
	)

	record := &korifiv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildGUID,
			Namespace: namespace,
			Labels: map[string]string{
				korifiv1alpha1.CFAppGUIDLabelKey: appGUID,
			},
		},
		Spec: korifiv1alpha1.CFBuildSpec{
			PackageRef: corev1.LocalObjectReference{
				Name: packageGUID,
			},
			AppRef: corev1.LocalObjectReference{
				Name: appGUID,
			},
			StagingMemoryMB: stagingMemory,
			StagingDiskMB:   stagingDisk,
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
				Data: korifiv1alpha1.LifecycleData{
					Buildpacks: []string{},
					Stack:      "",
				},
			},
		},
	}
	Expect(
		k8sClient.Create(ctx, record),
	).To(Succeed())
	return record
}

func createProcessCR(ctx context.Context, k8sClient client.Client, processGUID, spaceGUID, appGUID string) *korifiv1alpha1.CFProcess {
	toReturn := &korifiv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Name:      processGUID,
			Namespace: spaceGUID,
			Labels: map[string]string{
				korifiv1alpha1.CFAppGUIDLabelKey: appGUID,
			},
		},
		Spec: korifiv1alpha1.CFProcessSpec{
			AppRef: corev1.LocalObjectReference{
				Name: appGUID,
			},
			ProcessType: "web",
			Command:     "",
			HealthCheck: korifiv1alpha1.HealthCheck{
				Type: "process",
				Data: korifiv1alpha1.HealthCheckData{
					InvocationTimeoutSeconds: 0,
					TimeoutSeconds:           0,
				},
			},
			DesiredInstances: tools.PtrTo[int32](1),
			MemoryMB:         500,
			DiskQuotaMB:      512,
		},
	}
	Expect(
		k8sClient.Create(ctx, toReturn),
	).To(Succeed())
	return toReturn
}

func createDropletCR(ctx context.Context, k8sClient client.Client, dropletGUID, appGUID, spaceGUID string) *korifiv1alpha1.CFBuild {
	toReturn := &korifiv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dropletGUID,
			Namespace: spaceGUID,
		},
		Spec: korifiv1alpha1.CFBuildSpec{
			AppRef: corev1.LocalObjectReference{Name: appGUID},
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
			},
		},
	}
	Expect(
		k8sClient.Create(ctx, toReturn),
	).To(Succeed())
	return toReturn
}

func createServiceInstanceCR(ctx context.Context, k8sClient client.Client, serviceInstanceGUID, spaceGUID, name, secretName string) *korifiv1alpha1.CFServiceInstance {
	serviceInstance := &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:        serviceInstanceGUID,
			Namespace:   spaceGUID,
			Labels:      map[string]string{"a-label": "a-label-value"},
			Annotations: map[string]string{"an-annotation": "an-annotation-value"},
		},
		Spec: korifiv1alpha1.CFServiceInstanceSpec{
			DisplayName: name,
			SecretName:  secretName,
			Type:        "user-provided",
			Tags:        []string{"database", "mysql"},
		},
	}
	Expect(k8sClient.Create(ctx, serviceInstance)).To(Succeed())

	return serviceInstance
}

func createServiceBindingCR(ctx context.Context, k8sClient client.Client, serviceBindingGUID, spaceGUID string, name *string, serviceInstanceName, appName string) *korifiv1alpha1.CFServiceBinding {
	toReturn := &korifiv1alpha1.CFServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceBindingGUID,
			Namespace: spaceGUID,
		},
		Spec: korifiv1alpha1.CFServiceBindingSpec{
			DisplayName: name,
			Service: corev1.ObjectReference{
				Kind:       "ServiceInstance",
				Name:       serviceInstanceName,
				APIVersion: "korifi.cloudfoundry.org/v1alpha1",
			},
			AppRef: corev1.LocalObjectReference{
				Name: appName,
			},
		},
	}
	Expect(
		k8sClient.Create(ctx, toReturn),
	).To(Succeed())
	return toReturn
}

func createApp(space string) *korifiv1alpha1.CFApp {
	return createAppWithGUID(space, uuid.NewString())
}

func createAppWithGUID(space, guid string) *korifiv1alpha1.CFApp {
	cfApp := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: space,
			Annotations: map[string]string{
				CFAppRevisionKey: CFAppRevisionValue,
			},
		},
		Spec: korifiv1alpha1.CFAppSpec{
			DisplayName:  uuid.NewString(),
			DesiredState: "STOPPED",
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
				Data: korifiv1alpha1.LifecycleData{
					Buildpacks: []string{"java"},
				},
			},
			CurrentDropletRef: corev1.LocalObjectReference{
				Name: uuid.NewString(),
			},
			EnvSecretName: repositories.GenerateEnvSecretName(guid),
		},
	}
	Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())

	return cfApp
}
