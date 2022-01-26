package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Spaces", func() {
	Describe("creating spaces", func() {
		var (
			org       presenter.OrgResponse
			spaceName string
			space     presenter.SpaceResponse
		)

		BeforeEach(func() {
			space = presenter.SpaceResponse{}
			spaceName = generateGUID("space")
			org = createOrg(generateGUID("org"), adminAuthHeader)
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org.GUID, adminAuthHeader)
		})

		AfterEach(func() {
			deleteSubnamespace(rootNamespace, org.GUID)
		})

		When("admin", func() {
			JustBeforeEach(func() {
				space = createSpace(spaceName, org.GUID, adminAuthHeader)
			})

			It("creates a space", func() {
				Expect(space.Name).To(Equal(spaceName))

				Eventually(func() error {
					return k8sClient.Get(context.Background(), client.ObjectKey{Name: space.GUID}, &corev1.Namespace{})
				}).Should(Succeed())
			})

			When("the space name already exists", func() {
				It("returns an unprocessable entity error", func() {
					resp, err := createSpaceRaw(spaceName, org.GUID, adminAuthHeader)
					Expect(err).NotTo(HaveOccurred())
					defer resp.Body.Close()

					bodyMap := map[string]interface{}{}
					err = json.NewDecoder(resp.Body).Decode(&bodyMap)
					Expect(err).NotTo(HaveOccurred())

					Expect(resp).To(HaveHTTPStatus(http.StatusUnprocessableEntity))

					Expect(bodyMap).To(HaveKeyWithValue("errors", BeAssignableToTypeOf([]interface{}{})))
					errs := bodyMap["errors"].([]interface{})
					Expect(errs[0]).To(SatisfyAll(
						HaveKeyWithValue("code", BeNumerically("==", 10008)),
						HaveKeyWithValue("detail", MatchRegexp(fmt.Sprintf(`Space '%s' already exists.`, spaceName))),
						HaveKeyWithValue("title", Equal("CF-UnprocessableEntity")),
					))
				})
			})

			When("the organization relationship references a space guid", func() {
				It("denies the request", func() {
					resp, err := createSpaceRaw(generateGUID("some-other-space"), space.GUID, adminAuthHeader)
					Expect(err).NotTo(HaveOccurred())
					defer resp.Body.Close()

					bodyMap := map[string]interface{}{}
					err = json.NewDecoder(resp.Body).Decode(&bodyMap)
					Expect(err).NotTo(HaveOccurred())

					Expect(resp).To(HaveHTTPStatus(http.StatusUnprocessableEntity))

					Expect(bodyMap).To(HaveKeyWithValue("errors", BeAssignableToTypeOf([]interface{}{})))
					errs := bodyMap["errors"].([]interface{})
					Expect(errs[0]).To(SatisfyAll(
						HaveKeyWithValue("code", BeNumerically("==", 10008)),
						HaveKeyWithValue("detail", Equal("Invalid organization. Ensure the organization exists and you have access to it.")),
						HaveKeyWithValue("title", Equal("CF-UnprocessableEntity")),
					))
				})
			})
		})

		When("not admin", func() {
			It("returns a forbidden error", func() {
				resp, err := createSpaceRaw(spaceName, org.GUID, tokenAuthHeader)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp).To(HaveHTTPStatus(http.StatusForbidden))
			})
		})

		When("not authorized", func() {
			It("returns an unauthorized error", func() {
				resp, err := createSpaceRaw("shouldn't work", org.GUID, "")
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp).To(HaveHTTPStatus(http.StatusUnauthorized))
			})
		})
	})

	Describe("listing spaces", func() {
		var (
			org1, org2, org3          presenter.OrgResponse
			space11, space12, space13 presenter.SpaceResponse
			space21, space22, space23 presenter.SpaceResponse
			space31, space32, space33 presenter.SpaceResponse
		)

		BeforeEach(func() {
			var orgWG sync.WaitGroup
			orgErrChan := make(chan error, 3)

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

		It("lists the spaces the user has role in", func() {
			spaces, err := get(apis.SpaceListEndpoint, tokenAuthHeader)
			Expect(err).NotTo(HaveOccurred())
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

		When("not authorized", func() {
			BeforeEach(func() {
				tokenAuthHeader = ""
			})

			It("returns an unauthorized error", func() {
				_, err := get(apis.SpaceListEndpoint, tokenAuthHeader)
				Expect(err).To(MatchError(ContainSubstring(strconv.Itoa(http.StatusUnauthorized))))
			})
		})

		When("filtering by organization GUIDs", func() {
			It("only lists spaces beloging to the orgs", func() {
				spaces, err := getWithQuery(
					apis.SpaceListEndpoint,
					tokenAuthHeader,
					map[string]string{
						"organization_guids": fmt.Sprintf("%s,%s", org1.GUID, org3.GUID),
					},
				)
				Expect(err).NotTo(HaveOccurred())
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
