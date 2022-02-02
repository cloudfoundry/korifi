package e2e_test

import (
	"fmt"
	"net/http"
	"sync"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Spaces", func() {
	Describe("Creating spaces", func() {
		var (
			client    *resty.Client
			org       presenter.OrgResponse
			orgGUID   string
			spaceName string
			result    presenter.SpaceResponse
			resultErr map[string]interface{}
			httpResp  *resty.Response
			httpErr   error
		)

		BeforeEach(func() {
			spaceName = generateGUID("space")
			org = createOrg(generateGUID("org"))
			orgGUID = org.GUID
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org.GUID, adminAuthHeader)
		})

		AfterEach(func() {
			deleteSubnamespace(rootNamespace, org.GUID)
		})

		JustBeforeEach(func() {
			httpResp, httpErr = client.R().
				SetBody(fmt.Sprintf(`{"name": "%s", "relationships": {"organization": {"data": {"guid": "%s"}}}}`, spaceName, orgGUID)).
				SetError(&resultErr).
				SetResult(&result).
				Post("/v3/spaces")
		})

		When("admin", func() {
			BeforeEach(func() {
				client = adminClient
			})

			It("creates a space", func() {
				Expect(httpErr).NotTo(HaveOccurred())
				Expect(httpResp.StatusCode()).To(Equal(http.StatusCreated))
				Expect(result.Name).To(Equal(spaceName))
				Expect(result.GUID).NotTo(BeEmpty())
			})

			When("the space name already exists", func() {
				BeforeEach(func() {
					createSpace(spaceName, org.GUID)
				})

				It("returns an unprocessable entity error", func() {
					Expect(httpErr).NotTo(HaveOccurred())
					Expect(httpResp.StatusCode()).To(Equal(http.StatusUnprocessableEntity))
					Expect(resultErr).To(HaveKeyWithValue("errors", ConsistOf(
						SatisfyAll(
							HaveKeyWithValue("code", BeNumerically("==", 10008)),
							HaveKeyWithValue("detail", MatchRegexp(fmt.Sprintf(`Space '%s' already exists.`, spaceName))),
							HaveKeyWithValue("title", Equal("CF-UnprocessableEntity")),
						),
					)))
				})
			})

			When("the organization relationship references a space guid", func() {
				BeforeEach(func() {
					otherSpace := createSpace(generateGUID("some-other-space"), orgGUID)
					orgGUID = otherSpace.GUID
				})

				It("denies the request", func() {
					Expect(httpErr).NotTo(HaveOccurred())
					Expect(httpResp.StatusCode()).To(Equal(http.StatusUnprocessableEntity))
					Expect(resultErr).To(HaveKeyWithValue("errors", ConsistOf(
						SatisfyAll(
							HaveKeyWithValue("code", BeNumerically("==", 10008)),
							HaveKeyWithValue("detail", Equal("Invalid organization. Ensure the organization exists and you have access to it.")),
							HaveKeyWithValue("title", Equal("CF-UnprocessableEntity")),
						),
					)))
				})
			})
		})

		When("not admin", func() {
			BeforeEach(func() {
				client = tokenClient
			})

			It("returns a forbidden error", func() {
				Expect(httpErr).NotTo(HaveOccurred())
				Expect(httpResp.StatusCode()).To(Equal(http.StatusForbidden))
			})
		})
	})

	Describe("listing spaces", func() {
		var (
			org1, org2, org3          presenter.OrgResponse
			space11, space12, space13 presenter.SpaceResponse
			space21, space22, space23 presenter.SpaceResponse
			space31, space32, space33 presenter.SpaceResponse
			httpResp                  *resty.Response
			httpErr                   error
			spaces                    map[string]interface{}
			query                     map[string]string
		)

		BeforeEach(func() {
			var orgWG sync.WaitGroup
			orgErrChan := make(chan error, 3)
			query = make(map[string]string)

			orgWG.Add(3)
			asyncCreateOrg(generateGUID("org1"), adminAuthHeader, &org1, &orgWG, orgErrChan)
			asyncCreateOrg(generateGUID("org2"), adminAuthHeader, &org2, &orgWG, orgErrChan)
			asyncCreateOrg(generateGUID("org3"), adminAuthHeader, &org3, &orgWG, orgErrChan)
			orgWG.Wait()

			var err error
			Expect(orgErrChan).ToNot(Receive(&err), func() string { return fmt.Sprintf("unexpected error occurred while creating orgs: %v", err) })
			close(orgErrChan)

			var spaceWG sync.WaitGroup

			spaceErrChan := make(chan error, 9)
			spaceWG.Add(9)
			asyncCreateSpace(generateGUID("space11"), org1.GUID, adminAuthHeader, &space11, &spaceWG, spaceErrChan)
			asyncCreateSpace(generateGUID("space12"), org1.GUID, adminAuthHeader, &space12, &spaceWG, spaceErrChan)
			asyncCreateSpace(generateGUID("space13"), org1.GUID, adminAuthHeader, &space13, &spaceWG, spaceErrChan)

			asyncCreateSpace(generateGUID("space21"), org2.GUID, adminAuthHeader, &space21, &spaceWG, spaceErrChan)
			asyncCreateSpace(generateGUID("space22"), org2.GUID, adminAuthHeader, &space22, &spaceWG, spaceErrChan)
			asyncCreateSpace(generateGUID("space23"), org2.GUID, adminAuthHeader, &space23, &spaceWG, spaceErrChan)

			asyncCreateSpace(generateGUID("space31"), org3.GUID, adminAuthHeader, &space31, &spaceWG, spaceErrChan)
			asyncCreateSpace(generateGUID("space32"), org3.GUID, adminAuthHeader, &space32, &spaceWG, spaceErrChan)
			asyncCreateSpace(generateGUID("space33"), org3.GUID, adminAuthHeader, &space33, &spaceWG, spaceErrChan)
			spaceWG.Wait()

			Expect(spaceErrChan).ToNot(Receive(&err), func() string { return fmt.Sprintf("unexpected error occurred while creating spaces: %v", err) })
			close(spaceErrChan)

			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org1.GUID, adminAuthHeader)
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org2.GUID, adminAuthHeader)
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org3.GUID, adminAuthHeader)

			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space12.GUID, adminAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space11.GUID, adminAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space21.GUID, adminAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space22.GUID, adminAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space31.GUID, adminAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space32.GUID, adminAuthHeader)
		})

		AfterEach(func() {
			orgIDs := []string{org1.GUID, org2.GUID, org3.GUID}
			var wg sync.WaitGroup
			wg.Add(len(orgIDs))
			for _, id := range orgIDs {
				asyncDeleteSubnamespace(rootNamespace, id, &wg)
			}
			wg.Wait()
		})

		JustBeforeEach(func() {
			httpResp, httpErr = tokenClient.R().
				SetQueryParams(query).
				SetResult(&spaces).
				Get("/v3/spaces")
		})

		It("lists the spaces the user has role in", func() {
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(httpResp.StatusCode()).To(Equal(http.StatusOK))
			Expect(spaces).To(SatisfyAll(
				HaveKeyWithValue("pagination", HaveKeyWithValue("total_results", BeNumerically(">=", 6))),
				HaveKeyWithValue("resources", ContainElements(
					HaveKeyWithValue("name", space11.Name),
					HaveKeyWithValue("name", space12.Name),
					HaveKeyWithValue("name", space21.Name),
					HaveKeyWithValue("name", space22.Name),
					HaveKeyWithValue("name", space31.Name),
					HaveKeyWithValue("name", space32.Name),
				))))
			Expect(spaces).ToNot(
				HaveKeyWithValue("resources", ContainElements(
					HaveKeyWithValue("name", space13.Name),
					HaveKeyWithValue("name", space23.Name),
					HaveKeyWithValue("name", space33.Name),
				)))
		})

		When("filtering by organization GUIDs", func() {
			BeforeEach(func() {
				query = map[string]string{
					"organization_guids": fmt.Sprintf("%s,%s", org1.GUID, org3.GUID),
				}
			})

			It("only lists spaces beloging to the orgs", func() {
				Expect(httpErr).NotTo(HaveOccurred())
				Expect(httpResp.StatusCode()).To(Equal(http.StatusOK))
				Expect(spaces).To(
					HaveKeyWithValue("resources", ConsistOf(
						HaveKeyWithValue("name", space11.Name),
						HaveKeyWithValue("name", space12.Name),
						HaveKeyWithValue("name", space31.Name),
						HaveKeyWithValue("name", space32.Name),
					)))
			})
		})
	})
})
