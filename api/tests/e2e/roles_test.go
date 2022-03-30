package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Roles", func() {
	var (
		userName string
		resp     *resty.Response
		result   typedResource
		client   *resty.Client
	)

	BeforeEach(func() {
		userName = generateGUID("user")
		client = adminClient
		result = typedResource{}
	})

	Describe("creating an org role", func() {
		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetBody(typedResource{
					Type: "organization_manager",
					resource: resource{
						Relationships: relationships{
							"user":         {Data: resource{GUID: userName}},
							"organization": {Data: resource{GUID: commonTestOrgGUID}},
						},
					},
				}).
				SetResult(&result).
				Post("/v3/roles")
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates a role binding", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(result.GUID).ToNot(BeEmpty())
			Expect(result.Type).To(Equal("organization_manager"))
			Expect(result.Relationships).To(HaveKey("user"))
			Expect(result.Relationships["user"].Data.GUID).To(Equal(userName))
			Expect(result.Relationships).To(HaveKey("organization"))
			Expect(result.Relationships["organization"].Data.GUID).To(Equal(commonTestOrgGUID))
		})

		When("the user is not admin", func() {
			BeforeEach(func() {
				client = certClient
			})

			It("returns 403 Forbidden", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusForbidden))
			})
		})
	})

	Describe("creating a space role", func() {
		var spaceGUID string

		BeforeEach(func() {
			createOrgRole("organization_user", rbacv1.UserKind, userName, commonTestOrgGUID)
			spaceGUID = createSpace(uuid.NewString(), commonTestOrgGUID)
		})

		AfterEach(func() {
			deleteSpace(spaceGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetBody(typedResource{
					Type: "space_developer",
					resource: resource{
						Relationships: relationships{
							"user":  {Data: resource{GUID: userName}},
							"space": {Data: resource{GUID: spaceGUID}},
						},
					},
				}).
				SetResult(&result).
				Post("/v3/roles")
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates a role binding", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(result.GUID).ToNot(BeEmpty())
			Expect(result.Type).To(Equal("space_developer"))
			Expect(result.Relationships).To(HaveKey("user"))
			Expect(result.Relationships["user"].Data.GUID).To(Equal(userName))
			Expect(result.Relationships).To(HaveKey("space"))
			Expect(result.Relationships["space"].Data.GUID).To(Equal(spaceGUID))
		})

		When("the user is not admin", func() {
			BeforeEach(func() {
				client = certClient
			})

			It("returns 403 Forbidden", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusForbidden))
			})
		})
	})
})
