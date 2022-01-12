package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
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
			org            presenter.OrgResponse
			spaceName      string
			space          presenter.SpaceResponse
			extraSpaceGUID string
		)

		BeforeEach(func() {
			spaceName = generateGUID("space")
			org = createOrg(generateGUID("org"), adminAuthHeader)
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org.GUID, adminAuthHeader)
		})

		AfterEach(func() {
			ids := []string{}
			if space.GUID != "" {
				ids = append(ids, space.GUID)
			}

			if extraSpaceGUID != "" {
				ids = append(ids, extraSpaceGUID)
			}

			var wg sync.WaitGroup
			wg.Add(len(ids))
			for _, id := range ids {
				asyncDeleteSubnamespace(org.GUID, id, &wg)
			}
			wg.Wait()

			deleteSubnamespace(rootNamespace, org.GUID)
		})

		JustBeforeEach(func() {
			space = createSpace(spaceName, org.GUID, tokenAuthHeader)
		})

		It("creates a space", func() {
			Expect(space.Name).To(Equal(spaceName))

			Eventually(func() error {
				return k8sClient.Get(context.Background(), client.ObjectKey{Name: space.GUID}, &corev1.Namespace{})
			}).Should(Succeed())
		})

		When("the space name already exists", func() {
			It("returns an unprocessable entity error", func() {
				resp, err := createSpaceRaw(spaceName, org.GUID, tokenAuthHeader)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				bodyMap := map[string]interface{}{}
				err = json.NewDecoder(resp.Body).Decode(&bodyMap)
				Expect(err).NotTo(HaveOccurred())

				if resp.StatusCode == http.StatusCreated {
					extraSpaceGUID = bodyMap["guid"].(string)
				}

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
			org1 = createOrg(generateGUID("org1"), adminAuthHeader)
			org2 = createOrg(generateGUID("org2"), adminAuthHeader)
			org3 = createOrg(generateGUID("org3"), adminAuthHeader)

			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org1.GUID, adminAuthHeader)
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org2.GUID, adminAuthHeader)
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org3.GUID, adminAuthHeader)

			space11 = createSpace(generateGUID("space1"), org1.GUID, adminAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space11.GUID, adminAuthHeader)
			space12 = createSpace(generateGUID("space2"), org1.GUID, adminAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space12.GUID, adminAuthHeader)
			space13 = createSpace(generateGUID("space3"), org1.GUID, adminAuthHeader)

			space21 = createSpace(generateGUID("space1"), org2.GUID, adminAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space21.GUID, adminAuthHeader)
			space22 = createSpace(generateGUID("space2"), org2.GUID, adminAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space22.GUID, adminAuthHeader)
			space23 = createSpace(generateGUID("space3"), org2.GUID, adminAuthHeader)

			space31 = createSpace(generateGUID("space1"), org3.GUID, adminAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space31.GUID, adminAuthHeader)
			space32 = createSpace(generateGUID("space2"), org3.GUID, adminAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space32.GUID, adminAuthHeader)
			space33 = createSpace(generateGUID("space3"), org3.GUID, adminAuthHeader)
		})

		AfterEach(func() {
			var wg1 sync.WaitGroup
			ids := map[string]string{
				space11.GUID: org1.GUID, space12.GUID: org1.GUID, space13.GUID: org1.GUID,
				space21.GUID: org2.GUID, space22.GUID: org2.GUID, space23.GUID: org2.GUID,
				space31.GUID: org3.GUID, space32.GUID: org3.GUID, space33.GUID: org3.GUID,
			}
			wg1.Add(len(ids))
			for spaceID, orgID := range ids {
				asyncDeleteSubnamespace(orgID, spaceID, &wg1)
			}
			wg1.Wait()

			orgIDs := []string{org1.GUID, org2.GUID, org3.GUID}
			var wg2 sync.WaitGroup
			wg2.Add(len(orgIDs))
			for _, id := range orgIDs {
				asyncDeleteSubnamespace(rootNamespace, id, &wg2)
			}
			wg2.Wait()
		})

		It("lists the spaces the user has role in", func() {
			spaces, err := getSpaces(tokenAuthHeader)
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
				_, err := getSpaces(tokenAuthHeader)
				Expect(err).To(MatchError(ContainSubstring(strconv.Itoa(http.StatusUnauthorized))))
			})
		})

		When("filtering by organization GUIDs", func() {
			It("only lists spaces beloging to the orgs", func() {
				spaces, err := getSpacesWithQueryFn(tokenAuthHeader, map[string]string{"organization_guids": fmt.Sprintf("%s,%s", org1.GUID, org3.GUID)})
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

func getSpaces(authHeaderValue string) (map[string]interface{}, error) {
	return getSpacesWithQueryFn(authHeaderValue, nil)
}

func getSpacesWithQueryFn(authHeaderValue string, query map[string]string) (map[string]interface{}, error) {
	spacesUrl, err := url.Parse(apiServerRoot)
	if err != nil {
		return nil, err
	}

	spacesUrl.Path = apis.SpaceListEndpoint
	values := url.Values{}
	for key, val := range query {
		values.Set(key, val)
	}
	spacesUrl.RawQuery = values.Encode()

	resp, err := httpReq(http.MethodGet, spacesUrl.String(), authHeaderValue, nil)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	response := map[string]interface{}{}
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, err
	}

	return response, nil
}
