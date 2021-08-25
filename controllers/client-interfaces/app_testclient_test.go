package client_interfaces_test

import (
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/api/v1alpha1"
	client_interfaces "code.cloudfoundry.org/cf-k8s-controllers/controllers/client-interfaces"
	"context"
	"fmt"
	"k8s.io/client-go/kubernetes/scheme"
	"os"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

type CFAppClient interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object) error
	Status() client.StatusWriter
}

func TestAppClient(t *testing.T) {
	spec.Run(t, "object", testAppClient, spec.Report(report.Terminal{}))
}


func testAppClient(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)
	Expect := g.Expect
	logf.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.UseDevMode(true)))

	when("a Kubernetes client is initialized", func() {
		var (
			testEnv *envtest.Environment
			k8sClient client.Client
		)
		it.Before(func() {
			testEnv = &envtest.Environment{
					CRDDirectoryPaths:     []string{filepath.Join("../..", "config", "crd", "bases")},
					ErrorIfCRDPathMissing: true,
				}


			cfg, err := testEnv.Start()
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())

			err = workloadsv1alpha1.AddToScheme(scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())
			//+kubebuilder:scaffold:scheme

			k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient).NotTo(BeNil())
		})

		it("conforms to the CFAppClient interface", func() {
			var i interface{} = k8sClient
			_, ok := i.(CFAppClient)
			Expect(ok).To(BeTrue(), "Client did not cast to CFAppClient interface")
		})

		it.After(func() {
			err := testEnv.Stop()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	when("a dummy client is initialized", func() {
		fakeCFAppClient := client_interfaces.ShellCFAppClient{}
		it("conforms to the CFAppClient interface", func() {
			var i interface{} = &fakeCFAppClient
			_, ok := i.(CFAppClient)
			Expect(ok).To(BeTrue(), "Client did not cast to CFAppClient interface")
		})
	})

	when("Reconciling a CFApp where Get will fail", func() {
		fakeCFAppClient := client_interfaces.ShellCFAppClient{}
		fakeCFAppClient.GetFunc = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			return fmt.Errorf("Throws an error on purpose")
		}
		it("fails to reconcile", func() {

		})

	})
}