package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServicePlan", func() {
	DescribeTable("valid query",
		func(query string, expectedServicePlanList payloads.ServicePlanList) {
			actualServicePlanList, decodeErr := decodeQuery[payloads.ServicePlanList](query)

			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(*actualServicePlanList).To(Equal(expectedServicePlanList))
		},
		Entry("service_offering_guids", "service_offering_guids=b1,b2", payloads.ServicePlanList{ServiceOfferingGUIDs: "b1,b2"}),
	)

	Describe("ToMessage", func() {
		It("converts payload to repository message", func() {
			payload := &payloads.ServicePlanList{ServiceOfferingGUIDs: "b1,b2"}

			Expect(payload.ToMessage()).To(Equal(repositories.ListServicePlanMessage{
				ServiceOfferingGUIDs: []string{"b1", "b2"},
			}))
		})
	})
})
