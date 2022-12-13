package payloads_test

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AppList", func() {
	Describe("DecodeFromURLValues", func() {
		appList := payloads.AppList{}
		err := appList.DecodeFromURLValues(url.Values{
			"names":       []string{"name"},
			"guids":       []string{"guid"},
			"space_guids": []string{"space_guid"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(appList).To(Equal(payloads.AppList{
			Names:      "name",
			GUIDs:      "guid",
			SpaceGuids: "space_guid",
		}))
	})
})
