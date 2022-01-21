package repositories_test

import (
	"context"
	"path/filepath"
	"testing"

	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/api/tests/integration/helpers"
)

const (
	timeCheckThreshold = 3
)

func TestRepositories(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Repositories Suite")
}

var (
	testEnv          *envtest.Environment
	k8sClient        client.WithWatch
	k8sConfig        *rest.Config
	userName         string
	authInfo         authorization.Info
	rootNamespace    string
	idProvider       authorization.IdentityProvider
	nsPerms          *authorization.NamespacePermissions
	adminClusterRole *rbacv1.ClusterRole
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "controllers", "config", "crd", "bases"),
			filepath.Join("fixtures", "vendor", "hierarchical-namespaces", "config", "crd", "bases"),
		},
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "dependencies", "kpack-release-0.5.0.yaml"),
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
	err = hnsv1alpha2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = buildv1alpha2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.NewWithWatch(k8sConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	adminClusterRole = createAdminClusterRole(context.Background())
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
})

func createAnchorAndNamespace(ctx context.Context, inNamespace, name, orgSpaceLabel string) (*hnsv1alpha2.SubnamespaceAnchor, *corev1.Namespace) {
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

	return anchor, namespace
}

func createOrgAnchorAndNamespace(ctx context.Context, rootNamespace, name string) *hnsv1alpha2.SubnamespaceAnchor {
	org, _ := createAnchorAndNamespace(ctx, rootNamespace, name, repositories.OrgNameLabel)

	return org
}

func createSpaceAnchorAndNamespace(ctx context.Context, orgName, name string) *hnsv1alpha2.SubnamespaceAnchor {
	space, _ := createAnchorAndNamespace(ctx, orgName, name, repositories.SpaceNameLabel)

	return space
}

func createNamespace(ctx context.Context, orgName, name string) *corev1.Namespace {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				hnsv1alpha2.SubnamespaceOf: orgName,
			},
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

func createClusterRole(ctx context.Context, rules []rbacv1.PolicyRule) *rbacv1.ClusterRole {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: generateGUID(),
		},
		Rules: rules,
	}
	Expect(k8sClient.Create(ctx, clusterRole)).To(Succeed())

	return clusterRole
}

var (
	adminClusterRoleRules = []rbacv1.PolicyRule{
		{
			Verbs:     []string{"create", "delete"},
			APIGroups: []string{"rbac.authorization.k8s.io"},
			Resources: []string{"rolebindings"},
		},
		{
			Verbs:     []string{"create"},
			APIGroups: []string{"hnc.x-k8s.io"},
			Resources: []string{"subnamespaceanchors"},
		},
	}

	spaceDeveloperClusterRoleRules = []rbacv1.PolicyRule{
		{
			Verbs:     []string{"get", "patch"},
			APIGroups: []string{""},
			Resources: []string{"secrets"},
		},
		{
			Verbs:     []string{"get", "list", "create", "patch", "delete"},
			APIGroups: []string{"workloads.cloudfoundry.org"},
			Resources: []string{"cfapps"},
		},
		{
			Verbs:     []string{"get", "list", "create", "delete"},
			APIGroups: []string{"networking.cloudfoundry.org"},
			Resources: []string{"cfroutes"},
		},
		{
			Verbs:     []string{"get"},
			APIGroups: []string{"kpack.io"},
			Resources: []string{"clusterbuilders"},
		},
	}

	spaceAuditorClusterRoleRules = []rbacv1.PolicyRule{
		{
			Verbs:     []string{"get", "list"},
			APIGroups: []string{"workloads.cloudfoundry.org"},
			Resources: []string{"cfapps"},
		},
		{
			Verbs:     []string{"get"},
			APIGroups: []string{"kpack.io"},
			Resources: []string{"clusterbuilders"},
		},
	}

	orgManagerClusterRoleRules = []rbacv1.PolicyRule{
		{
			Verbs:     []string{"list", "delete"},
			APIGroups: []string{"hnc.x-k8s.io"},
			Resources: []string{"subnamespaceanchors"},
		},
		{
			Verbs:     []string{"get", "update"},
			APIGroups: []string{"hnc.x-k8s.io"},
			Resources: []string{"hierarchyconfigurations"},
		},
	}

	orgUserClusterRoleRules = []rbacv1.PolicyRule{}
)

func createAdminClusterRole(ctx context.Context) *rbacv1.ClusterRole {
	rules := []rbacv1.PolicyRule{}

	rules = append(rules, repositories.AdminClusterRoleRules...)
	rules = append(rules, repositories.SpaceDeveloperClusterRoleRules...)
	rules = append(rules, repositories.SpaceAuditorClusterRoleRules...)
	rules = append(rules, repositories.OrgManagerClusterRoleRules...)
	rules = append(rules, repositories.OrgUserClusterRoleRules...)

	return createClusterRole(ctx, rules)
}
