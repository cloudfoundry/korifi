package instance_index_injector_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"testing"

	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests/integration"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	arv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func TestInstanceIndexInjector(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "InstanceIndexInjector Suite")
}

var (
	fixture        *tests.Fixture
	eiriniBins     integration.EiriniBinaries
	config         *eirinictrl.ControllerConfig
	configFilePath string
	hookSession    *gexec.Session
	fingerprint    string
)

var _ = SynchronizedBeforeSuite(func() []byte {
	fixture = tests.NewFixture(GinkgoWriter)
	fixture.SetUp()

	port := int32(tests.GetTelepresencePort())
	telepresenceService := tests.GetTelepresenceServiceName()
	telepresenceDomain := fmt.Sprintf("%s.default.svc", telepresenceService)
	fingerprint = "instance-id-" + tests.GenerateGUID()[:8]

	eiriniBins = integration.NewEiriniBinaries()
	eiriniBins.EiriniController.Build()

	sideEffects := arv1.SideEffectClassNone
	scope := arv1.NamespacedScope

	_, err := fixture.Clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(context.Background(),
		&arv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: fingerprint + "-mutating-hook",
			},
			Webhooks: []arv1.MutatingWebhook{
				{
					Name: fingerprint + "-mutating-webhook.cloudfoundry.org",
					ClientConfig: arv1.WebhookClientConfig{
						Service: &arv1.ServiceReference{
							Namespace: "default",
							Name:      telepresenceService,
							Port:      &port,
						},
						CABundle: eiriniBins.CABundle,
					},
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1beta1"},
					Rules: []arv1.RuleWithOperations{
						{
							Operations: []arv1.OperationType{arv1.Create},
							Rule: arv1.Rule{
								APIGroups:   []string{""},
								APIVersions: []string{"v1"},
								Resources:   []string{"pods"},
								Scope:       &scope,
							},
						},
					},
				},
			},
		}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	config = integration.DefaultControllerConfig(fixture.Namespace)
	config.WebhookPort = port

	hookSession, configFilePath = eiriniBins.EiriniController.Run(config)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	Eventually(func() error {
		resp, err := client.Get(fmt.Sprintf("https://%s:%d/", telepresenceDomain, port))
		if err != nil {
			fmt.Fprintf(GinkgoWriter, "failed to call telepresence: %s", err.Error())

			return err
		}
		resp.Body.Close()

		return nil
	}, "2m", "500ms").Should(Succeed())

	return []byte{}
}, func(data []byte) {
	fixture = tests.NewFixture(GinkgoWriter)
})

var _ = SynchronizedAfterSuite(func() {
	fixture.Destroy()
}, func() {
	if configFilePath != "" {
		Expect(os.Remove(configFilePath)).To(Succeed())
	}
	if hookSession != nil {
		Eventually(hookSession.Kill()).Should(gexec.Exit())
	}
	err := fixture.Clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Delete(context.Background(), fingerprint+"-mutating-hook", metav1.DeleteOptions{})
	Expect(err).NotTo(HaveOccurred())

	eiriniBins.TearDown()
	fixture.Destroy()
})

var _ = BeforeEach(func() {
	fixture.SetUp()
})

var _ = AfterEach(func() {
	fixture.TearDown()
})
