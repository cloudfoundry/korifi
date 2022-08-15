package repositories_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/authorization/testhelpers"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

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
	timeCheckThreshold = 3
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
	k8sConfig             *rest.Config
	userName              string
	authInfo              authorization.Info
	rootNamespace         string
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
			filepath.Join("..", "..", "controllers", "config", "controllers", "crd", "bases"),
		},
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "dependencies", "kpack-release-0.6.0.yaml"),
			},
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	k8sConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sConfig).NotTo(BeNil())

	err = korifiv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = buildv1alpha2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.NewWithWatch(k8sConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	dynamicClient, err := dynamic.NewForConfig(k8sConfig)
	Expect(err).NotTo(HaveOccurred())
	Expect(dynamicClient).NotTo(BeNil())
	namespaceRetriever = repositories.NewNamespaceRetriver(dynamicClient)
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
	userName = generateGUID()
	cert, key := testhelpers.ObtainClientCert(testEnv, userName)
	authInfo.CertData = testhelpers.JoinCertAndKey(cert, key)
	rootNamespace = prefixedGUID("root-ns")
	tokenInspector := authorization.NewTokenReviewer(k8sClient)
	certInspector := authorization.NewCertInspector(k8sConfig)
	baseIDProvider := authorization.NewCertTokenIdentityProvider(tokenInspector, certInspector)
	idProvider = authorization.NewCachingIdentityProvider(baseIDProvider, cache.NewExpiring())
	nsPerms = authorization.NewNamespacePermissions(k8sClient, idProvider, rootNamespace)

	mapper, err := apiutil.NewDynamicRESTMapper(k8sConfig)
	Expect(err).NotTo(HaveOccurred())
	userClientFactory = authorization.NewUnprivilegedClientFactory(k8sConfig, mapper, authorization.NewDefaultBackoff())

	Expect(k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}})).To(Succeed())
	createRoleBinding(context.Background(), userName, rootNamespaceUserRole.Name, rootNamespace)
})

var _ = AfterEach(func() {
	Expect(k8sClient.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}})).To(Succeed())
})

func createOrgWithCleanup(ctx context.Context, name string) *korifiv1alpha1.CFOrg {
	guid := uuid.NewString()
	cfOrg := &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: rootNamespace,
		},
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: name,
		},
	}
	Expect(k8sClient.Create(ctx, cfOrg)).To(Succeed())

	meta.SetStatusCondition(&(cfOrg.Status.Conditions), metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "cus",
		Message: "cus",
	})
	Expect(k8sClient.Status().Update(ctx, cfOrg)).To(Succeed())

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfOrg.Name,
			Labels: map[string]string{
				korifiv1alpha1.OrgNameLabel: cfOrg.Spec.DisplayName,
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
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "cus",
		Message: "cus",
	})
	Expect(k8sClient.Status().Update(ctx, cfSpace)).To(Succeed())

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfSpace.Name,
			Labels: map[string]string{
				korifiv1alpha1.SpaceNameLabel: cfSpace.Spec.DisplayName,
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

func createNamespace(ctx context.Context, orgName, name string, labels map[string]string) *corev1.Namespace {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
	return namespace
}

func createClusterRole(ctx context.Context, filename string) *rbacv1.ClusterRole {
	filepath := filepath.Join("..", "..", "controllers", "config", "controllers", "cf_roles", filename+".yaml")
	content, err := os.ReadFile(filepath)
	Expect(err).NotTo(HaveOccurred())

	decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDecoder()
	clusterRole := &rbacv1.ClusterRole{}
	err = runtime.DecodeInto(decoder, content, clusterRole)
	Expect(err).NotTo(HaveOccurred())

	clusterRole.ObjectMeta.Name = "cf-" + clusterRole.ObjectMeta.Name
	Expect(k8sClient.Create(ctx, clusterRole)).To(Succeed())

	return clusterRole
}

func createRoleBinding(ctx context.Context, userName, roleName, namespace string) {
	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateGUID(),
			Namespace: namespace,
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
	Expect(k8sClient.Create(ctx, &roleBinding)).To(Succeed())
}
