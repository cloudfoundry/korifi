package payloads_test

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RouteList", func() {
	Describe("DecodeFromURLValues", func() {
		routeList := payloads.RouteList{}
		err := routeList.DecodeFromURLValues(url.Values{
			"app_guids":    []string{"app_guid"},
			"space_guids":  []string{"space_guid"},
			"domain_guids": []string{"domain_guid"},
			"hosts":        []string{"host"},
			"paths":        []string{"path"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(routeList).To(Equal(payloads.RouteList{
			AppGUIDs:    "app_guid",
			SpaceGUIDs:  "space_guid",
			DomainGUIDs: "domain_guid",
			Hosts:       "host",
			Paths:       "path",
		}))
	})
})
