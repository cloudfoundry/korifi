package e2e_test

import (
	"net/http"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Service Brokers", func() {
	var (
		resp *resty.Response
		err  error
	)

	Describe("Create", func() {
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

			jobURL := resp.Header().Get("Location")
			expectJobCompletes(resp)

			jobURLSplit := strings.Split(jobURL, "~")
			Expect(jobURLSplit).To(HaveLen(2))
			DeferCleanup(func() {
				cleanupBroker(jobURLSplit[1])
			})
		})
	})

	Describe("List", func() {
		var (
			result     resourceList[resource]
			brokerGUID string
		)

		BeforeEach(func() {
			brokerGUID = createBroker(serviceBrokerURL)
		})

		AfterEach(func() {
			cleanupBroker(brokerGUID)
		})

		JustBeforeEach(func() {
			resp, err = adminClient.R().SetResult(&result).Get("/v3/service_brokers")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns a list of brokers", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"GUID": Equal(brokerGUID),
			})))
		})
	})

	Describe("Update", func() {
		var brokerGUID string

		BeforeEach(func() {
			var jobURL string
			brokerGUID, jobURL = createBrokerAsync(serviceBrokerURL, "incorrect-username", "incorrect-password")
			Eventually(func(g Gomega) {
				jobResp, jobErr := adminClient.R().Get(jobURL)
				g.Expect(jobErr).NotTo(HaveOccurred())
				jobRespBody := string(jobResp.Body())
				g.Expect(jobRespBody).To(ContainSubstring("PROCESSING"))
			}).Should(Succeed())
		})

		AfterEach(func() {
			cleanupBroker(brokerGUID)
		})

		JustBeforeEach(func() {
			resp, err = adminClient.R().SetBody(map[string]any{
				"authentication": map[string]any{
					"type": "basic",
					"credentials": map[string]any{
						"username": "broker-user",
						"password": "broker-password",
					},
				},
			}).Patch("/v3/service_brokers/" + brokerGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds with job redirect", func() {
			Expect(resp).To(SatisfyAll(
				HaveRestyStatusCode(http.StatusAccepted),
				HaveRestyHeaderWithValue("Location", ContainSubstring("/v3/jobs/service_broker.update~")),
			))
			expectJobCompletes(resp)

			var servicePlans resourceList[resource]
			resp, err = adminClient.R().SetResult(&servicePlans).Get("/v3/service_plans")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(servicePlans.Resources).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Metadata": PointTo(MatchFields(IgnoreExtras, Fields{
					"Labels": HaveKeyWithValue(korifiv1alpha1.RelServiceBrokerGUIDLabel, brokerGUID),
				})),
			})))
		})
	})

	Describe("Delete", func() {
		var brokerGUID string

		BeforeEach(func() {
			brokerGUID = createBroker(serviceBrokerURL)
		})

		AfterEach(func() {
			cleanupBroker(brokerGUID)
		})

		JustBeforeEach(func() {
			resp, err = adminClient.R().Delete("/v3/service_brokers/" + brokerGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds with a job redirect", func() {
			Expect(resp).To(SatisfyAll(
				HaveRestyStatusCode(http.StatusAccepted),
				HaveRestyHeaderWithValue("Location", ContainSubstring("/v3/jobs/service_broker.delete~")),
			))
			expectJobCompletes(resp)
		})
	})
})
