/*

Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1_test

import (
	"code.cloudfoundry.org/cf-k8s-controllers/webhooks/workloads"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sclevine/spec"

	. "github.com/onsi/gomega"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	//+kubebuilder:scaffold:imports
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	suite     spec.Suite
	k8sClient client.Client
)

func Suite() spec.Suite {
	if suite == nil {
		suite = spec.New("MutatingWebhooks")
	}

	return suite
}

func AddToTestSuite(desc string, f func(t *testing.T, when spec.G, it spec.S)) bool {
	return Suite()(desc, f)
}

func TestSuite(t *testing.T) {
	g := NewWithT(t)

	testEnv, cancel := beforeSuite(g)
	defer afterSuite(g, testEnv, cancel)

	suite.Run(t)
}

func beforeSuite(g *WithT) (*envtest.Environment, context.CancelFunc) {
	logf.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.UseDevMode(true)))

	ctx, cancel := context.WithCancel(context.TODO())

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "..", "config", "webhook")},
		},
	}

	cfg, err := testEnv.Start()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg).NotTo(BeNil())

	scheme := runtime.NewScheme()
	err = workloadsv1alpha1.AddToScheme(scheme)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(admissionv1beta1.AddToScheme(scheme)).To(Succeed())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(k8sClient).NotTo(BeNil())

	// start webhook server using Manager
	webhookInstallOptions := &testEnv.WebhookInstallOptions
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		Host:               webhookInstallOptions.LocalServingHost,
		Port:               webhookInstallOptions.LocalServingPort,
		CertDir:            webhookInstallOptions.LocalServingCertDir,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect((&workloadsv1alpha1.CFApp{}).SetupWebhookWithManager(mgr)).To(Succeed())

	cfAppValidatingWebhook := &workloads.CFAppValidation{Client: mgr.GetClient()}
	g.Expect(cfAppValidatingWebhook.SetupWebhookWithManager(mgr)).To(Succeed())

	g.Expect((&workloadsv1alpha1.CFPackage{}).SetupWebhookWithManager(mgr)).To(Succeed())

	//+kubebuilder:scaffold:webhook

	go func() {
		err = mgr.Start(ctx)
		if err != nil {
			g.Expect(err).NotTo(HaveOccurred())
		}
	}()

	// wait for the webhook server to get ready
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	g.Eventually(func() error {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	}).Should(Succeed())
	return testEnv, cancel
}

func afterSuite(g *WithT, testEnv *envtest.Environment, cancel context.CancelFunc) {
	cancel() // call the cancel function to stop the controller context
	g.Expect(testEnv.Stop()).To(Succeed())
}
