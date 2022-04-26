package repositories_test

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/tests/integration/helpers"
	networkingv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/networking/v1alpha1"
	servicesv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/services/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"

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
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

const (
	timeCheckThreshold = 3
)

func TestRepositories(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Repositories Suite")
}

var (
	testEnv               *envtest.Environment
	k8sClient             client.WithWatch
	namespaceRetriever    repositories.NamespaceRetriever
	userClientFactory     repositories.UserK8sClientFactory
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
			filepath.Join("..", "..", "controllers", "config", "crd", "bases"),
			filepath.Join("..", "..", "controllers", "controllers", "workloads", "integration", "fixtures", "vendor", "hierarchical-namespaces", "config", "crd", "bases"),
		},
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "dependencies", "kpack-release-0.5.2.yaml"),
			},
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	k8sConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sConfig).NotTo(BeNil())

	err = workloadsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = networkingv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = servicesv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = hnsv1alpha2.AddToScheme(scheme.Scheme)
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

	ctx := context.Background()
	adminRole = createClusterRole(ctx, "cf_admin")
	orgManagerRole = createClusterRole(ctx, "cf_org_manager")
	orgUserRole = createClusterRole(ctx, "cf_org_user")
	spaceDeveloperRole = createClusterRole(ctx, "cf_space_developer")
	spaceManagerRole = createClusterRole(ctx, "cf_space_manager")
	spaceAuditorRole = createClusterRole(ctx, "cf_space_auditor")
	rootNamespaceUserRole = createClusterRole(ctx, "cf_root_namespace_user")
})

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).To(Succeed())
})

var _ = BeforeEach(func() {
	userName = generateGUID()
	cert, key := helpers.ObtainClientCert(testEnv, userName)
	authInfo.CertData = helpers.JoinCertAndKey(cert, key)
	rootNamespace = prefixedGUID("root-ns")
	tokenInspector := authorization.NewTokenReviewer(k8sClient)
	certInspector := authorization.NewCertInspector(k8sConfig)
	baseIDProvider := authorization.NewCertTokenIdentityProvider(tokenInspector, certInspector)
	idProvider = authorization.NewCachingIdentityProvider(baseIDProvider, cache.NewExpiring())
	nsPerms = authorization.NewNamespacePermissions(k8sClient, idProvider, rootNamespace)

	mapper, err := apiutil.NewDynamicRESTMapper(k8sConfig)
	Expect(err).NotTo(HaveOccurred())
	userClientFactory = repositories.NewUnprivilegedClientFactory(k8sConfig, mapper)

	Expect(k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}})).To(Succeed())
	createRoleBinding(context.Background(), userName, rootNamespaceUserRole.Name, rootNamespace)
})

var _ = AfterEach(func() {
	Expect(k8sClient.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}})).To(Succeed())
})

func createAnchorAndNamespace(ctx context.Context, inNamespace, name, orgSpaceLabel string) (*hnsv1alpha2.SubnamespaceAnchor, *corev1.Namespace, *hnsv1alpha2.HierarchyConfiguration) {
	guid := uuid.NewString()
	anchor := &hnsv1alpha2.SubnamespaceAnchor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: inNamespace,
			Labels:    map[string]string{orgSpaceLabel: name},
		},
		Status: hnsv1alpha2.SubnamespaceAnchorStatus{
			State: hnsv1alpha2.Ok,
		},
	}

	Expect(k8sClient.Create(ctx, anchor)).To(Succeed())

	depth := "1"
	if orgSpaceLabel == repositories.SpaceNameLabel {
		depth = "2"
	}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: anchor.Name,
			Labels: map[string]string{
				rootNamespace + hnsv1alpha2.LabelTreeDepthSuffix: depth,
			},
			Annotations: map[string]string{
				hnsv1alpha2.SubnamespaceOf: inNamespace,
			},
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

	hierarchy := &hnsv1alpha2.HierarchyConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hierarchy",
			Namespace: guid,
		},
		Spec: hnsv1alpha2.HierarchyConfigurationSpec{
			Parent: inNamespace,
		},
	}
	Expect(k8sClient.Create(ctx, hierarchy)).To(Succeed())

	return anchor, namespace, hierarchy
}

func createOrgWithCleanup(ctx context.Context, name string) *workloadsv1alpha1.CFOrg {
	guid := uuid.NewString()
	cfOrg := &workloadsv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: rootNamespace,
		},
		Spec: workloadsv1alpha1.CFOrgSpec{
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
				rootNamespace + hnsv1alpha2.LabelTreeDepthSuffix: "1",
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

func createSpaceWithCleanup(ctx context.Context, orgName, name string) *hnsv1alpha2.SubnamespaceAnchor {
	space, namespace, hierarchy := createAnchorAndNamespace(ctx, orgName, name, repositories.SpaceNameLabel)

	DeferCleanup(func() {
		Expect(k8sClient.Delete(ctx, hierarchy)).To(Succeed())
		Expect(k8sClient.Delete(ctx, space)).To(Succeed())
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	return space
}

func createSpaceAnchorAndNamespace(ctx context.Context, orgName, name string) *hnsv1alpha2.SubnamespaceAnchor {
	space, _, _ := createAnchorAndNamespace(ctx, orgName, name, repositories.SpaceNameLabel)

	return space
}

func createNamespace(ctx context.Context, orgName, name string, labels map[string]string) *corev1.Namespace {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				hnsv1alpha2.SubnamespaceOf: orgName,
			},
			Labels: labels,
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
	return namespace
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

func createClusterRoleBinding(ctx context.Context, userName, roleName string) {
	clusterRoleBinding := rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: generateGUID(),
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
	Expect(k8sClient.Create(ctx, &clusterRoleBinding)).To(Succeed())
}

func createClusterRole(ctx context.Context, filename string) *rbacv1.ClusterRole {
	filepath := filepath.Join("..", "..", "controllers", "config", "cf_roles", filename+".yaml")
	content, err := ioutil.ReadFile(filepath)
	Expect(err).NotTo(HaveOccurred())

	decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDecoder()
	clusterRole := &rbacv1.ClusterRole{}
	err = runtime.DecodeInto(decoder, content, clusterRole)
	Expect(err).NotTo(HaveOccurred())

	clusterRole.ObjectMeta.Name = "cf-" + clusterRole.ObjectMeta.Name
	Expect(k8sClient.Create(ctx, clusterRole)).To(Succeed())

	return clusterRole
}
