package e2e_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/payloads"
	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Security Group", func() {
	var (
		resp              *resty.Response
		securityGroupName string
		securityGroupGUID string
	)

	Describe("Create", func() {
		var (
			respResource securityGroupResource
			resultErr    cfErrs
		)

		BeforeEach(func() {
			securityGroupName = generateGUID("create")
			resultErr = cfErrs{}
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetBody(securityGroupResource{
					Name: securityGroupName,
					Rules: []payloads.SecurityGroupRule{
						{
							Protocol:    "tcp",
							Ports:       "80",
							Destination: "192.168.1.1",
						},
					},
				}).
				SetResult(&respResource).
				SetError(&resultErr).
				Post("/v3/security_groups")
			Expect(err).NotTo(HaveOccurred())

			securityGroupGUID = respResource.GUID
		})

		It("creates the domain", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(respResource.GUID).To(Equal(securityGroupGUID))
			Expect(respResource.Name).To(Equal(securityGroupName))
			Expect(respResource.Rules).To(Equal([]payloads.SecurityGroupRule{
				{
					Protocol:    "tcp",
					Ports:       "80",
					Destination: "192.168.1.1",
				},
			}))
		})
	})
})
