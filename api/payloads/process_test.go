package payloads_test

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ProcessList", func() {
	Describe("DecodeFromURLValues", func() {
		processList := payloads.ProcessList{}
		err := processList.DecodeFromURLValues(url.Values{
			"app_guids": []string{"app_guid"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(processList).To(Equal(payloads.ProcessList{
			AppGUIDs: "app_guid",
		}))
	})
})
