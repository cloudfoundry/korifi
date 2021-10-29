package authorization_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestAuthorization(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Authorization Suite")
}

var (
	testEnv   *envtest.Environment
	k8sClient client.Client
	k8sConfig *rest.Config
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{}

	var err error
	k8sConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sConfig).NotTo(BeNil())

	k8sClient, err = client.New(k8sConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).To(Succeed())
})
