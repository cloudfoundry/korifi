package e2e_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/tests/helpers/broker"
	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Service Offerings", func() {
	var (
		resp       *resty.Response
		brokerGUID string
	)

	BeforeEach(func() {
		brokerGUID = createBroker(serviceBrokerURL)
	})

	AfterEach(func() {
		broker.NewCatalogDeleter(rootNamespace).ForBrokerGUID(brokerGUID).Delete()
	})

	Describe("List", func() {
		var result resourceList[resource]

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().SetResult(&result).Get("/v3/service_offerings")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns service offerings", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Relationships": HaveKeyWithValue("service_broker", relationship{
					Data: resource{
						GUID: brokerGUID,
					},
				}),
			})))
		})
	})
})
