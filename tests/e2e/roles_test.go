package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Roles", func() {
	var (
		userName string
		resp     *resty.Response
		result   typedResource
	)

	BeforeEach(func() {
		userName = generateGUID("user")
		result = typedResource{}
	})

	Describe("creating an org role", func() {
		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
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

		It("creates a role", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(result.GUID).ToNot(BeEmpty())
			Expect(result.Type).To(Equal("organization_manager"))
			Expect(result.Relationships).To(HaveKey("user"))
			Expect(result.Relationships["user"].Data.GUID).To(Equal(userName))
			Expect(result.Relationships).To(HaveKey("organization"))
			Expect(result.Relationships["organization"].Data.GUID).To(Equal(commonTestOrgGUID))
		})
	})

	Describe("creating a space role", func() {
		var spaceGUID string

		BeforeEach(func() {
			createOrgRole("organization_user", userName, commonTestOrgGUID)
			spaceGUID = createSpace(uuid.NewString(), commonTestOrgGUID)
		})

		AfterEach(func() {
			deleteSpace(spaceGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
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

		It("creates a role", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(result.GUID).ToNot(BeEmpty())
			Expect(result.Type).To(Equal("space_developer"))
			Expect(result.Relationships).To(HaveKey("user"))
			Expect(result.Relationships["user"].Data.GUID).To(Equal(userName))
			Expect(result.Relationships).To(HaveKey("space"))
			Expect(result.Relationships["space"].Data.GUID).To(Equal(spaceGUID))
		})
	})

	Describe("listing roles", func() {
		var (
			spaceGUID  string
			resultList resourceList[typedResource]
		)

		BeforeEach(func() {
			createOrgRole("organization_user", userName, commonTestOrgGUID)
			spaceGUID = createSpace(uuid.NewString(), commonTestOrgGUID)
			createSpaceRole("space_developer", userName, spaceGUID)
		})

		AfterEach(func() {
			deleteSpace(spaceGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&resultList).
				Get("/v3/roles")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns roles", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(resultList.Resources).To(ContainElements(
				SatisfyAll(
					HaveRelationship("user", "GUID", userName),
					HaveRelationship("organization", "GUID", commonTestOrgGUID),
					MatchFields(IgnoreExtras, Fields{
						"Type": Equal("organization_user"),
					}),
				),
				SatisfyAll(
					HaveRelationship("user", "GUID", userName),
					HaveRelationship("space", "GUID", spaceGUID),
					MatchFields(IgnoreExtras, Fields{
						"Type": Equal("space_developer"),
					}),
				),
			))
		})
	})

	Describe("deleting a role", func() {
		var (
			spaceGUID string
			roleGUID  string
		)

		BeforeEach(func() {
			createOrgRole("organization_user", userName, commonTestOrgGUID)
			spaceGUID = createSpace(uuid.NewString(), commonTestOrgGUID)
			roleGUID = createSpaceRole("space_developer", userName, spaceGUID)
		})

		AfterEach(func() {
			deleteSpace(spaceGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&result).
				Delete("/v3/roles/" + roleGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds with a job redirect", func() {
			Expect(resp).To(SatisfyAll(
				HaveRestyStatusCode(http.StatusAccepted),
				HaveRestyHeaderWithValue("Location", HaveSuffix("/v3/jobs/role.delete~"+roleGUID)),
			))

			jobURL := resp.Header().Get("Location")
			Eventually(func(g Gomega) {
				jobResp, err := adminClient.R().Get(jobURL)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(string(jobResp.Body())).To(ContainSubstring("COMPLETE"))
			}).Should(Succeed())

			resp, err := adminClient.R().Get("/v3/roles/" + roleGUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(HaveRestyStatusCode(http.StatusNotFound))
		})
	})
})
