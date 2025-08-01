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
		respResource      securityGroupResource
		resultErr         cfErrs
		err               error
	)

	BeforeEach(func() {
		securityGroupName = generateGUID("create")
		resultErr = cfErrs{}

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
		Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
		securityGroupGUID = respResource.GUID
	})

	Describe("Get", func() {
		JustBeforeEach(func() {
			resp, err = adminClient.R().SetResult(&respResource).Get("/v3/security_groups/" + securityGroupGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("retrieves the security group", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(respResource.GUID).To(Equal(securityGroupGUID))
			Expect(respResource.Name).To(Equal(securityGroupName))
		})
	})

	Describe("Create", func() {
		It("creates the security group", func() {
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

	Describe("Bind", func() {
		var (
			bindResp  relationshipDataResource
			spaceGUID string
		)

		JustBeforeEach(func() {
			spaceGUID = createSpace(generateGUID("space1"), commonTestOrgGUID)
			resp, err = adminClient.R().
				SetBody(relationshipDataResource{
					Data: []payloads.RelationshipData{{
						GUID: spaceGUID,
					}},
				}).
				SetResult(&bindResp).
				SetError(&resultErr).
				Post("/v3/security_groups/" + securityGroupGUID + "/relationships/running_spaces")

			Expect(err).NotTo(HaveOccurred())
		})

		It("binds the space to the security group", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(bindResp.Data).To(ContainElement(payloads.RelationshipData{
				GUID: spaceGUID,
			}))
		})
	})
})
