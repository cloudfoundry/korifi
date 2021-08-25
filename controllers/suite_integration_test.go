// +build integration

package controllers_test

import (
	"os"
	"path/filepath"

	. "code.cloudfoundry.org/cf-k8s-controllers/controllers"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/api/v1alpha1"
	//+kubebuilder:scaffold:imports
	"testing"
)

var (
	suite     spec.Suite
	testEnv   *envtest.Environment
	k8sClient client.Client
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
	Suite().Before(func(t *testing.T) {
		g := NewWithT(t)
		//logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
		logf.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.UseDevMode(true)))

		testEnv = &envtest.Environment{
			CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
			ErrorIfCRDPathMissing: true,
		}

		cfg, err := testEnv.Start()
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cfg).NotTo(BeNil())

		err = workloadsv1alpha1.AddToScheme(scheme.Scheme)
		g.Expect(err).NotTo(HaveOccurred())
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

		// TODO: Add the other reconcilers

		go func() {
			err = k8sManager.Start(ctrl.SetupSignalHandler())
			g.Expect(err).ToNot(HaveOccurred())
		}()
	})

	Suite().After(func(t *testing.T) {
		g := NewWithT(t)
		err := testEnv.Stop()
		g.Expect(err).NotTo(HaveOccurred())
	})

	suite.Run(t)
}
