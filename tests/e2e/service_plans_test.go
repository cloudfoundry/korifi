package e2e_test

import (
	"net/http"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
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
})
