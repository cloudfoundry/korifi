package e2e_test

import (
	"net/http"
	"strings"

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
			Eventually(func(g Gomega) {
				resp, err = adminClient.R().Get(jobURL)
				g.Expect(err).NotTo(HaveOccurred())
				jobRespBody := string(resp.Body())
				g.Expect(jobRespBody).To(ContainSubstring("COMPLETE"))
			}).Should(Succeed())

			jobURLSplit := strings.Split(jobURL, "~")
			Expect(jobURLSplit).To(HaveLen(2))
			DeferCleanup(func() {
				cleanupBroker(jobURLSplit[1])
			})
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

			jobURL := resp.Header().Get("Location")
			Eventually(func(g Gomega) {
				resp, err = adminClient.R().Get(jobURL)
				g.Expect(err).NotTo(HaveOccurred())
				jobRespBody := string(resp.Body())
				g.Expect(jobRespBody).To(ContainSubstring("COMPLETE"))
			}).Should(Succeed())
		})
	})
})
