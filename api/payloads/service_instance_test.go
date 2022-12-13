package payloads_test

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceInstanceList", func() {
	Describe("DecodeFromURLValues", func() {
		serviceInstanceList := payloads.ServiceInstanceList{}
		err := serviceInstanceList.DecodeFromURLValues(url.Values{
			"names":       []string{"name"},
			"space_guids": []string{"space_guid"},
			"order_by":    []string{"order"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(serviceInstanceList).To(Equal(payloads.ServiceInstanceList{
			Names:      "name",
			SpaceGuids: "space_guid",
			OrderBy:    "order",
		}))
	})
})
