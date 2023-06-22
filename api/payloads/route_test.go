package payloads_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RouteList", func() {
	Describe("decode from url values", func() {
		It("succeeds", func() {
			routeList := payloads.RouteList{}
			req, err := http.NewRequest("GET", "http://foo.com/bar?app_guids=app_guid&space_guids=space_guid&domain_guids=domain_guid&hosts=host&paths=path", nil)
			Expect(err).NotTo(HaveOccurred())
			err = validator.DecodeAndValidateURLValues(req, &routeList)

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
})
