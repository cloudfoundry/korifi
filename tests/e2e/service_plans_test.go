package e2e_test

import (
	"fmt"
	"net/http"

	"code.cloudfoundry.org/korifi/api/payloads"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/helpers/broker"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Service Plans", func() {
	var (
		brokerGUID string
		resp       *resty.Response
		err        error
	)

	BeforeEach(func() {
		brokerGUID = createBroker(serviceBrokerURL)
	})

	AfterEach(func() {
		broker.NewDeleter(rootNamespace).ForBrokerGUID(brokerGUID).Delete()
	})

	Describe("List", func() {
		var result resourceList[resource]

		JustBeforeEach(func() {
			resp, err = adminClient.R().SetResult(&result).Get("/v3/service_plans")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns service plans", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Metadata": PointTo(MatchFields(IgnoreExtras, Fields{
					"Labels": HaveKeyWithValue(korifiv1alpha1.RelServiceBrokerGUIDLabel, brokerGUID),
				})),
			})))
		})
	})
	Describe("Get", func() {
		var (
			planGUID string
			result   servicePlanResource
		)

		BeforeEach(func() {
			plans := resourceList[resource]{}
			resp, err = adminClient.R().SetResult(&plans).Get("/v3/service_plans")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

			brokerPlans := itx.FromSlice(plans.Resources).Filter(func(r resource) bool {
				return r.Metadata.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel] == brokerGUID
			}).Collect()

			Expect(brokerPlans).NotTo(BeEmpty())
			planGUID = brokerPlans[0].GUID
		})

		It("returns the service plan", func() {
			resp, err = adminClient.R().SetResult(&result).Get(fmt.Sprintf("/v3/service_plans/%s", planGUID))
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.GUID).To(Equal(planGUID))
		})
	})

	Describe("Visibility", func() {
		var (
			planGUID string
			result   planVisibilityResource
		)

		BeforeEach(func() {
			plans := resourceList[resource]{}
			resp, err = adminClient.R().SetResult(&plans).Get("/v3/service_plans")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

			brokerPlans := itx.FromSlice(plans.Resources).Filter(func(r resource) bool {
				return r.Metadata.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel] == brokerGUID
			}).Collect()

			Expect(brokerPlans).NotTo(BeEmpty())
			planGUID = brokerPlans[0].GUID
		})

		Describe("Get Visibility", func() {
			JustBeforeEach(func() {
				resp, err = adminClient.R().SetResult(&result).Get(fmt.Sprintf("/v3/service_plans/%s/visibility", planGUID))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns the service plan visibility", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result).To(Equal(planVisibilityResource{
					Type: "public",
				}))
			})
		})

		Describe("Apply Visibility", func() {
			JustBeforeEach(func() {
				resp, err = adminClient.R().
					SetResult(&result).
					SetBody(planVisibilityResource{
						Type: "admin",
					}).
					Post(fmt.Sprintf("/v3/service_plans/%s/visibility", planGUID))
				Expect(err).NotTo(HaveOccurred())
			})

			It("applies the plan visibility", func() {
				Expect(resp).To(SatisfyAll(
					HaveRestyStatusCode(http.StatusOK),
					HaveRestyBody(MatchJSON(`{
						"type": "admin"
					}`)),
				))
			})
		})

		Describe("Update Visibility", func() {
			JustBeforeEach(func() {
				resp, err = adminClient.R().
					SetResult(&result).
					SetBody(planVisibilityResource{
						Type: "admin",
					}).
					Patch(fmt.Sprintf("/v3/service_plans/%s/visibility", planGUID))
				Expect(err).NotTo(HaveOccurred())
			})

			It("updates the plan visibility", func() {
				Expect(resp).To(SatisfyAll(
					HaveRestyStatusCode(http.StatusOK),
					HaveRestyBody(MatchJSON(`{
						"type": "admin"
					}`)),
				))
			})
		})

		Describe("Delete Visibility", func() {
			BeforeEach(func() {
				resp, err = adminClient.R().
					SetBody(planVisibilityResource{
						Type: "organization",
						Organizations: []payloads.VisibilityOrganization{{
							GUID: "org-guid",
						}},
					}).
					Post(fmt.Sprintf("/v3/service_plans/%s/visibility", planGUID))
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(SatisfyAll(
					HaveRestyStatusCode(http.StatusOK),
				))
			})

			JustBeforeEach(func() {
				resp, err = adminClient.R().
					SetResult(&result).
					Delete(fmt.Sprintf("/v3/service_plans/%s/visibility/%s", planGUID, "org-guid"))
				Expect(err).NotTo(HaveOccurred())
			})

			It("deletes the plan visibility", func() {
				Expect(resp).To(SatisfyAll(
					HaveRestyStatusCode(http.StatusNoContent),
				))
			})
		})
	})

	Describe("Delete", func() {
		var servicePlanGUID string

		BeforeEach(func() {
			plans := resourceList[resource]{}
			resp, err = adminClient.R().SetResult(&plans).Get("/v3/service_plans?service_broker_guids=" + brokerGUID)
			Expect(err).ToNot(HaveOccurred())

			servicePlanGUID = plans.Resources[0].GUID
			resp, err = adminClient.R().Delete("/v3/service_plans/" + servicePlanGUID)
		})

		It("deletes the service plan", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(resp).To(HaveRestyStatusCode(http.StatusNoContent))

			resp, err = adminClient.R().Get("/v3/service_plans/" + servicePlanGUID)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp).To(HaveRestyStatusCode(http.StatusNotFound))
		})
	})
})
