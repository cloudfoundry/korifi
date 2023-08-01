package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Buildpacks", func() {
	Describe("list", func() {
		var (
			result resourceList[responseResource]
			resp   *resty.Response
		)

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&result).
				Get("/v3/buildpacks")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns a list of buildpacks", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(ContainElements(
				MatchFields(IgnoreExtras, Fields{"Name": ContainSubstring("java")}),
			))
		})
	})
})
