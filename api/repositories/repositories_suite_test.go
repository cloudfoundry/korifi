package repositories_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/api/tests/integration/helpers"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
)

const (
	timeCheckThreshold = 3
)

func TestRepositories(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Repositories Suite")
}

var (
	testEnv   *envtest.Environment
	k8sClient client.WithWatch
	k8sConfig *rest.Config
	userName  string
	authInfo  authorization.Info
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "controllers", "config", "crd", "bases"),
			filepath.Join("fixtures", "vendor", "hierarchical-namespaces", "config", "crd", "bases"),
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

	k8sClient, err = client.NewWithWatch(k8sConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	userName = generateGUID()
	cert, key := helpers.ObtainClientCert(testEnv, userName)
	authInfo.CertData = helpers.JoinCertAndKey(cert, key)
})

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).To(Succeed())
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

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: anchor.Name,
			Annotations: map[string]string{
				hnsv1alpha2.SubnamespaceOf: inNamespace,
			},
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

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

func createSpaceDeveloperClusterRole(ctx context.Context) *rbacv1.ClusterRole {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: generateGUID(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"list"},
				APIGroups: []string{"workloads.cloudfoundry.org"},
				Resources: []string{"cfapps"},
			},
		},
	}
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
