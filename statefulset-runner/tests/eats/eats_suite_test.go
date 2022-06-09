package eats_test

import (
	"testing"
	"time"

	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/dynamic"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
)

func TestEats(t *testing.T) {
	SetDefaultEventuallyTimeout(2 * time.Minute)
	RegisterFailHandler(tests.EatsFailHandler)
	RunSpecs(t, "Eats Suite")
}

var fixture *tests.EATSFixture

var _ = SynchronizedBeforeSuite(
	func() []byte {
		return nil
	},

	func(_ []byte) {
		baseFixture := tests.NewFixture(GinkgoWriter)
		config, err := clientcmd.BuildConfigFromFlags("", baseFixture.KubeConfigPath)
		Expect(err).NotTo(HaveOccurred(), "failed to build config from flags")

		dynamicClientset, err := dynamic.NewForConfig(config)
		Expect(err).NotTo(HaveOccurred(), "failed to create clientset")

		fixture = tests.NewEATSFixture(*baseFixture, dynamicClientset)
	},
)

var _ = AfterSuite(func() {
	fixture.Destroy()
})

var _ = BeforeEach(func() {
	fixture.SetUp()
})

var _ = AfterEach(func() {
	fixture.TearDown()
})
