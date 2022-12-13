package payloads_test

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceBindingList", func() {
	Describe("DecodeFromURLValues", func() {
		serviceBindingList := payloads.ServiceBindingList{}
		err := serviceBindingList.DecodeFromURLValues(url.Values{
			"app_guids":              []string{"app_guid"},
			"service_instance_guids": []string{"service_instance_guid"},
			"include":                []string{"include"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(serviceBindingList).To(Equal(payloads.ServiceBindingList{
			AppGUIDs:             "app_guid",
			ServiceInstanceGUIDs: "service_instance_guid",
			Include:              "include",
		}))
	})
})
