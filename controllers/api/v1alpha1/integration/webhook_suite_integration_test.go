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

package integration_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/coordination"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/networking"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

const (
	defaultMemoryMB    = 128
	defaultDiskQuotaMB = 256
	defaultTimeout     = 60
)

var (
	cancel          context.CancelFunc
	testEnv         *envtest.Environment
	k8sClient       client.Client
	cfRootNamespace string
)

func TestKorifiMutatingWebhooks(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Korifi Mutating Webhooks Integration Test Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancelFunc := context.WithCancel(context.TODO())
	cancel = cancelFunc

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: false,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "..", "config", "webhook")},
		},
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	scheme := runtime.NewScheme()
	Expect(korifiv1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(admissionv1beta1.AddToScheme(scheme)).To(Succeed())
	Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

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
	Expect(err).NotTo(HaveOccurred())

	Expect((&korifiv1alpha1.CFApp{}).SetupWebhookWithManager(mgr)).To(Succeed())
	Expect((&korifiv1alpha1.CFRoute{}).SetupWebhookWithManager(mgr)).To(Succeed())

	Expect(workloads.NewCFAppValidator(
		webhooks.NewDuplicateValidator(coordination.NewNameRegistry(mgr.GetClient(), workloads.AppEntityType)),
	).SetupWebhookWithManager(mgr)).To(Succeed())
	Expect(networking.NewCFDomainValidator(mgr.GetClient()).SetupWebhookWithManager(mgr)).To(Succeed())

	cfRootNamespace = "default"
	Expect(networking.NewCFRouteValidator(
		webhooks.NewDuplicateValidator(coordination.NewNameRegistry(mgr.GetClient(), networking.RouteEntityType)),
		cfRootNamespace,
		mgr.GetClient(),
	).SetupWebhookWithManager(mgr)).To(Succeed())

	Expect((&korifiv1alpha1.CFPackage{}).SetupWebhookWithManager(mgr)).To(Succeed())

	Expect(korifiv1alpha1.NewCFProcessDefaulter(defaultMemoryMB, defaultDiskQuotaMB, defaultTimeout).
		SetupWebhookWithManager(mgr)).To(Succeed())

	Expect((&korifiv1alpha1.CFBuild{}).SetupWebhookWithManager(mgr)).To(Succeed())

	//+kubebuilder:scaffold:webhook

	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	}()

	// wait for the webhook server to get ready
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	Eventually(func() error {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	}).Should(Succeed())
})

var _ = AfterSuite(func() {
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

func getMapKeyValue(m map[string]string, k string) string {
	if m == nil {
		return ""
	}
	if v, has := m[k]; has {
		return v
	}
	return ""
}
