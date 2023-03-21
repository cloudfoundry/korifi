package cleanup_test

import (
	"context"
	"path/filepath"
	"testing"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gcustom"
	gtypes "github.com/onsi/gomega/types"
	"go.uber.org/zap/zapcore"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestCleanup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cleanup Suite")
}

var (
	ctx       context.Context
	testEnv   *envtest.Environment
	k8sClient client.Client
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "helm", "korifi", "controllers", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sClient, err = client.New(cfg, client.Options{})
	Expect(err).NotTo(HaveOccurred())

	ctx = context.Background()
})

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).To(Succeed())
})

func BeNotFound() gtypes.GomegaMatcher {
	return gcustom.MakeMatcher(func(obj client.Object) (bool, error) {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		return k8serrors.IsNotFound(err), nil
	}).WithTemplate("Expected:\n{{.Actual.Namespace}}/{{.Actual.Name}}\n{{.To}} be not found")
}

func BeFound() gtypes.GomegaMatcher {
	return gcustom.MakeMatcher(func(obj client.Object) (bool, error) {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		return err == nil, nil
	}).WithTemplate("Expected:\n{{.Actual.Namespace}}/{{.Actual.Name}}\n{{.To}} be found")
}
