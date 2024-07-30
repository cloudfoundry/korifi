package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceOffering", func() {
	DescribeTable("valid query",
		func(query string, expectedServiceOfferingList payloads.ServiceOfferingList) {
			actualServiceOfferingList, decodeErr := decodeQuery[payloads.ServiceOfferingList](query)

			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(*actualServiceOfferingList).To(Equal(expectedServiceOfferingList))
		},
		Entry("names", "names=b1,b2", payloads.ServiceOfferingList{Names: "b1,b2"}),
	)

	Describe("ToMessage", func() {
		It("converts payload to repository message", func() {
			payload := &payloads.ServiceOfferingList{Names: "b1,b2"}

			Expect(payload.ToMessage()).To(Equal(repositories.ListServiceOfferingMessage{
				Names: []string{"b1", "b2"},
			}))
		})
	})
})
