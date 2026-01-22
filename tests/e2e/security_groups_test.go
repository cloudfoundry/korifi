package e2e_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tests/helpers/security_group"
	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Security Group", func() {
	var (
		resp              *resty.Response
		securityGroupName string
	)

	Describe("Create", func() {
		var (
			securityGroupGUID string
			respResource      securityGroupResource
			resultErr         cfErrs
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
			DeferCleanup(func() {
				security_group.Delete(rootNamespace, securityGroupGUID)
			})
		})

		It("creates the security group", func() {
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
