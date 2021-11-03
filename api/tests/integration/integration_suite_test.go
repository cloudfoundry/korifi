package integration_test

import (
	"context"
	"testing"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/tests/integration/helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const oidcPrefix string = "oidc:"

var (
	testEnv      *envtest.Environment
	k8sClient    client.Client
	k8sConfig    *rest.Config
	authProvider *helpers.AuthProvider
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	SetDefaultEventuallyTimeout(10 * time.Second)
})

var _ = BeforeEach(func() {
	authProvider = helpers.NewAuthProvider()
	startEnvTest(authProvider.APIServerExtraArgs(oidcPrefix)...)
})

var _ = AfterEach(func() {
	authProvider.Stop()
	Expect(testEnv.Stop()).To(Succeed())
})

func startEnvTest(apiServerEtraArgs ...string) {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	apiServerArgs := append(envtest.DefaultKubeAPIServerFlags, apiServerEtraArgs...)

	testEnv = &envtest.Environment{
		AttachControlPlaneOutput: false, // set to true for full apiserver logs
		KubeAPIServerFlags:       apiServerArgs,
	}

	var err error
	k8sConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	authv1.AddToScheme(scheme.Scheme)

	k8sClient, err = client.New(k8sConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	namespaceList := &corev1.NamespaceList{}
	Eventually(func() error {
		return k8sClient.List(context.Background(), namespaceList)
	}).Should(Succeed())
}

func restartEnvTest(apiServerEtraArgs ...string) {
	Expect(testEnv.Stop()).To(Succeed())
	startEnvTest(apiServerEtraArgs...)
}
