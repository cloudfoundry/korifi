package workloads_test

import (
	cfconfig "code.cloudfoundry.org/cf-k8s-controllers/config/cf"
	"os"
	"path/filepath"
	"testing"

	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads"
	. "github.com/onsi/gomega"
	buildv1alpha1 "github.com/pivotal/kpack/pkg/apis/build/v1alpha1"
	"github.com/sclevine/spec"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	//+kubebuilder:scaffold:imports
)

var (
	k8sClient client.Client
	suite     spec.Suite
	testEnv   *envtest.Environment
)

func Suite() spec.Suite {
	if suite == nil {
		suite = spec.New("Controllers")
	}

	return suite
}

func AddToTestSuite(desc string, f func(t *testing.T, when spec.G, it spec.S)) bool {
	return Suite()(desc, f)
}

func TestSuite(t *testing.T) {
	g := NewWithT(t)

	testEnv := beforeSuite(g)
	defer afterSuite(g, testEnv)

	suite.Run(t)
}

func afterSuite(g *WithT, testEnv *envtest.Environment) {
	g.Expect(testEnv.Stop()).To(Succeed())
}

func beforeSuite(g *WithT) *envtest.Environment {
	logf.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{filepath.Join("..", "..", "dependencies", "kpack-release-0.3.1.yaml")},
		},
	}

	cfg, err := testEnv.Start()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg).NotTo(BeNil())

	g.Expect(workloadsv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	g.Expect(buildv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(k8sClient).NotTo(BeNil())
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	g.Expect(err).ToNot(HaveOccurred())

	err = (&CFAppReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
		Log:    ctrl.Log.WithName("controllers").WithName("CFApp"),
	}).SetupWithManager(k8sManager)
	g.Expect(err).ToNot(HaveOccurred())

	err = (&CFBuildReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
		Log:    ctrl.Log.WithName("controllers").WithName("CFBuild"),
		ControllerConfig: &cfconfig.ControllerConfig{
			KpackImageTag: "image/registry/tag",
		},
	}).SetupWithManager(k8sManager)
	g.Expect(err).ToNot(HaveOccurred())

	// TODO: Add the other reconcilers

	go func() {
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		g.Expect(err).ToNot(HaveOccurred())
	}()
	return testEnv
}
