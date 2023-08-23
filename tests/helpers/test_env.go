package helpers

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func NewCachedClient(cfg *rest.Config) (client.Client, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	clientCache, err := cache.New(cfg, cache.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		Expect(clientCache.Start(ctx)).To(Succeed())
	}()
	clientCache.WaitForCacheSync(ctx)

	k8sClient, err := client.New(cfg, client.Options{
		Scheme: scheme.Scheme,
		Cache:  &client.CacheOptions{Reader: clientCache},
	})
	Expect(err).NotTo(HaveOccurred())

	return NewSyncClient(k8sClient), cancel
}

func NewK8sManager(testEnv *envtest.Environment, managerRolePath string) manager.Manager {
	k8sManager, err := ctrl.NewManager(SetupTestEnvUser(testEnv, managerRolePath), ctrl.Options{
		Scheme: scheme.Scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    testEnv.WebhookInstallOptions.LocalServingHost,
			Port:    testEnv.WebhookInstallOptions.LocalServingPort,
			CertDir: testEnv.WebhookInstallOptions.LocalServingCertDir,
		}),
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},

		LeaderElection: false,
	})
	Expect(err).NotTo(HaveOccurred())

	return k8sManager
}

func StartK8sManager(k8sManager manager.Manager) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer GinkgoRecover()
		Expect(k8sManager.Start(ctx)).To(Succeed())
	}()

	Eventually(func(g Gomega) {
		g.Expect(k8sManager.GetWebhookServer().StartedChecker()(nil)).To(Succeed())
	}).Should(Succeed())

	return cancel
}

func SetupTestEnvUser(testEnv *envtest.Environment, roleDefinitionPath string) *rest.Config {
	userName := uuid.NewString()
	controllersUser, err := testEnv.ControlPlane.AddUser(envtest.User{Name: userName}, testEnv.Config)
	Expect(err).NotTo(HaveOccurred())

	adminClient, err := client.New(testEnv.Config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	controllersRole := ensureClusterRole(adminClient, roleDefinitionPath)
	bindUserToClusterRole(adminClient, userName, controllersRole.Name)

	return controllersUser.Config()
}

func bindUserToClusterRole(k8sClient client.Client, userName, roleName string) {
	GinkgoHelper()

	Expect(k8sClient.Create(context.Background(), &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: userName,
		},
		Subjects: []rbacv1.Subject{{
			Kind: "User",
			Name: userName,
		}},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: roleName,
		},
	})).To(Succeed())
}

func ensureClusterRole(k8sClient client.Client, roleDefinitionPath string) *rbacv1.ClusterRole {
	clusterRoleDefinition, err := os.ReadFile(toAbsPath(roleDefinitionPath))
	Expect(err).NotTo(HaveOccurred())

	roleObject, _, err := scheme.Codecs.UniversalDeserializer().Decode(clusterRoleDefinition, nil, new(rbacv1.ClusterRole))
	Expect(err).NotTo(HaveOccurred())

	clusterRole, ok := roleObject.(*rbacv1.ClusterRole)
	Expect(ok).To(BeTrue())

	Expect(client.IgnoreAlreadyExists(k8sClient.Create(context.Background(), clusterRole))).To(Succeed())

	return clusterRole
}

func toAbsPath(roleDefinitionPath string) string {
	_, thisFilePath, _, ok := runtime.Caller(0)
	Expect(ok).To(BeTrue())
	thisFileDir := filepath.Dir(thisFilePath)

	return filepath.Join(thisFileDir, "..", "..", roleDefinitionPath)
}
