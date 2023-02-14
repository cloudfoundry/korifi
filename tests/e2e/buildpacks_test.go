package e2e_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/tests/e2e/helpers"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Buildpacks", func() {
	var restyClient *helpers.CorrelatedRestyClient

	BeforeEach(func() {
		restyClient = certClient
	})

	Describe("list", func() {
		var (
			result resourceList[responseResource]
			resp   *resty.Response
		)

		JustBeforeEach(func() {
			var err error
			resp, err = restyClient.R().
				SetResult(&result).
				Get("/v3/buildpacks")
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user has acquired the cf_user role", func() {
			It("returns a list of buildpacks", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Resources).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{"Name": ContainSubstring("java")}),
				))
			})
		})

		When("the user has no permissions", func() {
			BeforeEach(func() {
				restyClient = tokenClient
			})

			It("returns forbidden", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusForbidden))
			})
		})
	})
})
