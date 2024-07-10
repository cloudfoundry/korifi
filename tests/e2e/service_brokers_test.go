package e2e_test

import (
	"context"
	"net/http"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Service Brokers", func() {
	var (
		resp *resty.Response
		err  error
	)

	Describe("Create", func() {
		BeforeEach(func() {
			Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		})

		JustBeforeEach(func() {
			resp, err = adminClient.R().
				SetBody(serviceBrokerResource{
					resource: resource{
						Name: uuid.NewString(),
					},
					URL: serviceBrokerURL,
					Authentication: serviceBrokerAuthenticationResource{
						Type: "basic",
						Credentials: map[string]any{
							"username": "broker-user",
							"password": "broker-password",
						},
					},
				}).
				Post("/v3/service_brokers")
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds with a job redirect", func() {
			Expect(resp).To(SatisfyAll(
				HaveRestyStatusCode(http.StatusAccepted),
				HaveRestyHeaderWithValue("Location", ContainSubstring("/v3/jobs/service_broker.create~")),
			))

			locationSplit := strings.Split(resp.Header().Get("Location"), "~")
			DeferCleanup(func() {
				// Temporarily delete the broker created by the test via the k8s client
				// Once the API supports broker deletion, we could get rid of this
				if len(locationSplit) != 2 {
					return
				}

				var config *rest.Config
				config, err = controllerruntime.GetConfig()
				Expect(err).NotTo(HaveOccurred())

				var k8sClient client.Client
				k8sClient, err = client.New(config, client.Options{Scheme: scheme.Scheme})
				Expect(err).NotTo(HaveOccurred())

				broker := &korifiv1alpha1.CFServiceBroker{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: rootNamespace,
						Name:      locationSplit[1],
					},
				}

				Expect(k8sClient.Delete(context.Background(), broker)).To(Succeed())
			})

			jobURL := resp.Header().Get("Location")
			Eventually(func(g Gomega) {
				resp, err = adminClient.R().Get(jobURL)
				g.Expect(err).NotTo(HaveOccurred())
				jobRespBody := string(resp.Body())
				g.Expect(jobRespBody).To(ContainSubstring("COMPLETE"))
			}).Should(Succeed())
		})
	})

	Describe("List", func() {
		var result resourceList[resource]

		JustBeforeEach(func() {
			resp, err = adminClient.R().SetResult(&result).Get("/v3/service_brokers")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns a list of brokers", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"GUID": Equal(serviceBrokerGUID),
			})))
		})
	})
})
