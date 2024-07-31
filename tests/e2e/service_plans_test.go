package e2e_test

import (
	"fmt"
	"net/http"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/BooleanCat/go-functional/iter"
	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Service Plans", func() {
	var resp *resty.Response

	Describe("List", func() {
		var result resourceList[resource]

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().SetResult(&result).Get("/v3/service_plans")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns service plans", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Metadata": PointTo(MatchFields(IgnoreExtras, Fields{
					"Labels": HaveKeyWithValue(korifiv1alpha1.RelServiceBrokerLabel, serviceBrokerGUID),
				})),
			})))
		})
	})

	Describe("Get Visibility", func() {
		var (
			result   planVisibilityResource
			planGUID string
		)

		BeforeEach(func() {
			plans := resourceList[resource]{}

			listResp, err := adminClient.R().SetResult(&plans).Get("/v3/service_plans")
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp).To(HaveRestyStatusCode(http.StatusOK))

			brokerPlans := iter.Lift(plans.Resources).Filter(func(r resource) bool {
				return r.Metadata.Labels[korifiv1alpha1.RelServiceBrokerLabel] == serviceBrokerGUID
			}).Collect()

			Expect(brokerPlans).NotTo(BeEmpty())
			planGUID = brokerPlans[0].GUID
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().SetResult(&result).Get(fmt.Sprintf("/v3/service_plans/%s/visibility", planGUID))
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the service plan visibility", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result).To(Equal(planVisibilityResource{
				Type: korifiv1alpha1.AdminServicePlanVisibilityType,
			}))
		})
	})
})
