package integration_test

import (
	"context"
	"testing"
	"time"

	"code.cloudfoundry.org/korifi/tests/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestIntegration(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var (
	testEnv           *envtest.Environment
	k8sClient         client.Client
	controllersClient client.Client
	cacheStop         context.CancelFunc
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{}

	adminConfig, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(adminConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	controllersConf := helpers.SetupControllersUser(testEnv)
	userCache, err := cache.New(controllersConf, cache.Options{})
	Expect(err).NotTo(HaveOccurred())

	var cacheCtx context.Context
	cacheCtx, cacheStop = context.WithCancel(context.Background())
	go func() {
		GinkgoRecover()
		Expect(userCache.Start(cacheCtx)).To(Succeed())
	}()
	userCache.WaitForCacheSync(cacheCtx)

	controllersClient, err = client.New(
		controllersConf,
		client.Options{
			Scheme: scheme.Scheme,
			Cache: &client.CacheOptions{
				Reader: userCache,
			},
		},
	)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	cacheStop()
	Expect(testEnv.Stop()).To(Succeed())
})
