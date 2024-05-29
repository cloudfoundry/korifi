package controllers_test

import (
	"testing"

	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestAppWorkloadsController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var (
	fakeClient       *fake.Client
	fakeStatusWriter *fake.StatusWriter
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))
})

var _ = BeforeEach(func() {
	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	fakeClient = new(fake.Client)
	fakeStatusWriter = &fake.StatusWriter{}
	fakeClient.StatusReturns(fakeStatusWriter)
})
