package payloads_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceBindingList", func() {
	Describe("decode from url values", func() {
		It("succeeds", func() {
			serviceBindingList := payloads.ServiceBindingList{}
			req, err := http.NewRequest("GET", "http://foo.com/bar?app_guids=app_guid&service_instance_guids=service_instance_guid&include=include", nil)
			Expect(err).NotTo(HaveOccurred())
			err = validator.DecodeAndValidateURLValues(req, &serviceBindingList)

			Expect(err).NotTo(HaveOccurred())
			Expect(serviceBindingList).To(Equal(payloads.ServiceBindingList{
				AppGUIDs:             "app_guid",
				ServiceInstanceGUIDs: "service_instance_guid",
				Include:              "include",
			}))
		})
	})
})
