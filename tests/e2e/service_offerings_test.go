package e2e_test

import (
	"net/http"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/helpers/broker"
	"github.com/BooleanCat/go-functional/v2/it/itx"
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
		broker.NewDeleter(rootNamespace).ForBrokerGUID(brokerGUID).Delete()
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

	Describe("PATCH /v3/service_offerings/{guid}", func() {
		var (
			err          error
			respResource responseResource
			offeringGUID string
		)

		BeforeEach(func() {
			offerings := resourceList[resource]{}
			resp, err = adminClient.R().SetResult(&offerings).Get("/v3/service_offerings")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

			serviceOfferings := itx.FromSlice(offerings.Resources).Filter(func(r resource) bool {
				return r.Metadata.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel] == brokerGUID
			}).Collect()

			Expect(serviceOfferings).NotTo(BeEmpty())
			offeringGUID = serviceOfferings[0].GUID
		})

		JustBeforeEach(func() {
			resp, err = adminClient.R().
				SetBody(metadataResource{
					Metadata: &metadataPatch{
						Annotations: &map[string]string{"foo": "bar"},
						Labels:      &map[string]string{"baz": "bar"},
					},
				}).
				SetResult(&respResource).
				Patch("/v3/service_offerings/" + offeringGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns 200 OK and updates service offering labels and annotations", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(respResource.GUID).To(Equal(offeringGUID))
			Expect(respResource.Metadata.Annotations).To(HaveKeyWithValue("foo", "bar"))
			Expect(respResource.Metadata.Labels).To(HaveKeyWithValue("baz", "bar"))
		})
	})
})
