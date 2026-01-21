package repositories_test

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/authorization/testhelpers"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/common_labels"
	"code.cloudfoundry.org/korifi/controllers/webhooks/label_indexer"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tools/k8s"
	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/client-go/discovery"
	k8sclient "k8s.io/client-go/kubernetes"

	routesdestwebhook "code.cloudfoundry.org/korifi/controllers/webhooks/networking/routes/app_destinations"
	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
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
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Repositories Suite")
}

var (
	ctx                          context.Context
	testEnv                      *envtest.Environment
	k8sClient                    client.WithWatch
	userClientFactory            authorization.UnprivilegedClientFactory
	spaceScopedUserClientFactory authorization.UserClientFactory
	spaceScopedKlient            repositories.Klient
	rootNSKlient                 repositories.Klient
	userName                     string
	authInfo                     authorization.Info
	rootNamespace                string
	builderName                  string
	runnerName                   string
	idProvider                   authorization.IdentityProvider
	nsPerms                      *authorization.NamespacePermissions
	adminRole                    *rbacv1.ClusterRole
	spaceDeveloperRole           *rbacv1.ClusterRole
	spaceManagerRole             *rbacv1.ClusterRole
	orgManagerRole               *rbacv1.ClusterRole
	orgUserRole                  *rbacv1.ClusterRole
	spaceAuditorRole             *rbacv1.ClusterRole
	rootNamespaceUserRole        *rbacv1.ClusterRole
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	webhooksManifest := helpers.GenerateWebhookManifest(
		"code.cloudfoundry.org/korifi/controllers/webhooks/common_labels",
		"code.cloudfoundry.org/korifi/controllers/webhooks/label_indexer",
		"code.cloudfoundry.org/korifi/controllers/webhooks/networking/routes/app_destinations",
	)

	DeferCleanup(func() {
		Expect(os.RemoveAll(filepath.Dir(webhooksManifest))).To(Succeed())
	})

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "helm", "korifi", "controllers", "crds"),
			filepath.Join("..", "..", "tests", "vendor", "kpack"),
		},
		ErrorIfCRDPathMissing: true,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{webhooksManifest},
		},
	}

	_, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(buildv1alpha2.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sManager := helpers.NewK8sManager(testEnv, filepath.Join("helm", "korifi", "controllers", "role.yaml"))

	common_labels.NewWebhook().SetupWebhookWithManager(k8sManager)
	label_indexer.NewWebhook().SetupWebhookWithManager(k8sManager)
	routesdestwebhook.NewRouteAppDestinationsWebhook().SetupWebhookWithManager(k8sManager)

	DeferCleanup(helpers.StartK8sManager(k8sManager))

	clientScheme := runtime.NewScheme()
	Expect(korifiv1alpha1.AddToScheme(clientScheme)).To(Succeed())
	Expect(buildv1alpha2.AddToScheme(clientScheme)).To(Succeed())
	Expect(rbacv1.AddToScheme(clientScheme)).To(Succeed())
	Expect(corev1.AddToScheme(clientScheme)).To(Succeed())
	metav1.AddToGroupVersion(clientScheme, metav1.SchemeGroupVersion)
	Expect(metav1.AddMetaToScheme(clientScheme)).To(Succeed())
	Expect(authv1.SchemeBuilder.AddToScheme(clientScheme)).To(Succeed())

	k8sClient, err = client.NewWithWatch(testEnv.Config, client.Options{Scheme: clientScheme})
	Expect(err).NotTo(HaveOccurred())

	adminRole = createClusterRole(context.Background(), "cf_admin")
	orgManagerRole = createClusterRole(context.Background(), "cf_org_manager")
	orgUserRole = createClusterRole(context.Background(), "cf_org_user")
	spaceDeveloperRole = createClusterRole(context.Background(), "cf_space_developer")
	spaceManagerRole = createClusterRole(context.Background(), "cf_space_manager")
	spaceAuditorRole = createClusterRole(context.Background(), "cf_space_auditor")
	rootNamespaceUserRole = createClusterRole(context.Background(), "cf_root_namespace_user")
})

var _ = AfterSuite(func() {
	Eventually(testEnv.Stop, "1m").Should(Succeed())
})

var _ = BeforeEach(func() {
	ctx = context.Background()

	ctx = logr.NewContext(ctx, stdr.New(log.New(GinkgoWriter, ">>>", log.LstdFlags)))

	userName = uuid.NewString()
	cert, key := testhelpers.ObtainClientCert(testEnv, userName)
	authInfo.CertData = testhelpers.JoinCertAndKey(cert, key)
	ctx = authorization.NewContext(ctx, &authInfo)

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

	dynamicClient, err := dynamic.NewForConfig(testEnv.Config)
	Expect(err).NotTo(HaveOccurred())
	namespaceRetriever := repositories.NewNamespaceRetriever(dynamicClient)

	clientset, err := k8sclient.NewForConfig(testEnv.Config)
	Expect(err).NotTo(HaveOccurred())

	restClient := clientset.RESTClient()
	pluralizer := descriptors.NewCachingPluralizer(discovery.NewDiscoveryClient(restClient))

	userClientFactory = authorization.NewUnprivilegedClientFactory(testEnv.Config, mapper, k8sClient.Scheme()).
		WithWrappingFunc(func(client client.WithWatch) client.WithWatch {
			return k8s.NewRetryingClient(client, k8s.IsForbidden, k8s.NewDefaultBackoff())
		})

	spaceScopedDescriptorsClient := descriptors.NewClient(restClient, pluralizer, k8sClient.Scheme(), authorization.NewSpaceFilteringOpts(nsPerms))
	spaceScopedUserClientFactory = userClientFactory.WithWrappingFunc(func(client client.WithWatch) client.WithWatch {
		return authorization.NewSpaceFilteringClient(client, k8sClient, authorization.NewSpaceFilteringOpts(nsPerms))
	})
	spaceScopedObjectListMapper := descriptors.NewObjectListMapper(spaceScopedUserClientFactory)
	spaceScopedKlient = k8sklient.NewK8sKlient(
		namespaceRetriever,
		spaceScopedUserClientFactory,
		k8sklient.NewDescriptorsBasedLister(spaceScopedDescriptorsClient, spaceScopedObjectListMapper),
		k8sClient.Scheme(),
	)

	rootNsUserClientFactory := userClientFactory.WithWrappingFunc(func(client client.WithWatch) client.WithWatch {
		return authorization.NewRootNSFilteringClient(client, rootNamespace)
	})
	rootNSObjectListMapper := descriptors.NewObjectListMapper(rootNsUserClientFactory)
	rootNsDescriptorsClient := descriptors.NewClient(restClient, pluralizer, k8sClient.Scheme(), authorization.NewRootNsFilteringOpts(rootNamespace))
	rootNSKlient = k8sklient.NewK8sKlient(
		namespaceRetriever,
		rootNsUserClientFactory,
		k8sklient.NewDescriptorsBasedLister(rootNsDescriptorsClient, rootNSObjectListMapper),
		k8sClient.Scheme())

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
	Expect(k8s.Patch(ctx, k8sClient, cfOrg, func() {
		meta.SetStatusCondition(&cfOrg.Status.Conditions, metav1.Condition{
			Type:   korifiv1alpha1.StatusConditionReady,
			Status: metav1.ConditionTrue,
			Reason: "Ready",
		})
		cfOrg.Status.GUID = guid
	})).To(Succeed())

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfOrg.Name,
			Labels: map[string]string{
				korifiv1alpha1.CFOrgDisplayNameKey: cfOrg.Labels[korifiv1alpha1.CFOrgDisplayNameKey],
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
	Expect(k8s.Patch(ctx, k8sClient, cfSpace, func() {
		meta.SetStatusCondition(&cfSpace.Status.Conditions, metav1.Condition{
			Type:   korifiv1alpha1.StatusConditionReady,
			Status: metav1.ConditionTrue,
			Reason: "Ready",
		})
		cfSpace.Status.GUID = guid
	})).To(Succeed())

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfSpace.Name,
			Labels: map[string]string{
				korifiv1alpha1.CFSpaceDisplayNameKey: cfSpace.Labels[korifiv1alpha1.CFSpaceDisplayNameKey],
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

func createApp(space string) *korifiv1alpha1.CFApp {
	return createAppWithGUID(space, uuid.NewString())
}

func createAppWithGUID(space, guid string) *korifiv1alpha1.CFApp {
	GinkgoHelper()

	displayName := uuid.NewString()
	cfApp := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: space,
			Annotations: map[string]string{
				CFAppRevisionKey: CFAppRevisionValue,
			},
		},
		Spec: korifiv1alpha1.CFAppSpec{
			DisplayName:  displayName,
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
			EnvSecretName: uuid.NewString(),
		},
	}
	Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())

	return cfApp
}
