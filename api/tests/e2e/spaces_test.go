package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	. "github.com/onsi/ginkgo"
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
			org = createOrg(generateGUID("org"), tokenAuthHeader)
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org.GUID, tokenAuthHeader)
		})

		AfterEach(func() {
			if space.GUID != "" {
				deleteSubnamespace(org.GUID, space.GUID)
				waitForNamespaceDeletion(org.GUID, space.GUID)
			}

			if extraSpaceGUID != "" {
				deleteSubnamespace(org.GUID, extraSpaceGUID)
				waitForNamespaceDeletion(org.GUID, extraSpaceGUID)
			}

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
			org1 = createOrg(generateGUID("org1"), tokenAuthHeader)
			org2 = createOrg(generateGUID("org2"), tokenAuthHeader)
			org3 = createOrg(generateGUID("org3"), tokenAuthHeader)

			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org1.GUID, tokenAuthHeader)
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org2.GUID, tokenAuthHeader)
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org3.GUID, tokenAuthHeader)

			space11 = createSpace(generateGUID("space1"), org1.GUID, tokenAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space11.GUID, tokenAuthHeader)
			space12 = createSpace(generateGUID("space2"), org1.GUID, tokenAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space12.GUID, tokenAuthHeader)
			space13 = createSpace(generateGUID("space3"), org1.GUID, tokenAuthHeader)

			space21 = createSpace(generateGUID("space1"), org2.GUID, tokenAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space21.GUID, tokenAuthHeader)
			space22 = createSpace(generateGUID("space2"), org2.GUID, tokenAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space22.GUID, tokenAuthHeader)
			space23 = createSpace(generateGUID("space3"), org2.GUID, tokenAuthHeader)

			space31 = createSpace(generateGUID("space1"), org3.GUID, tokenAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space31.GUID, tokenAuthHeader)
			space32 = createSpace(generateGUID("space2"), org3.GUID, tokenAuthHeader)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space32.GUID, tokenAuthHeader)
			space33 = createSpace(generateGUID("space3"), org3.GUID, tokenAuthHeader)
		})

		AfterEach(func() {
			deleteSubnamespace(org1.GUID, space11.GUID)
			deleteSubnamespace(org1.GUID, space12.GUID)
			deleteSubnamespace(org1.GUID, space13.GUID)
			deleteSubnamespace(org2.GUID, space21.GUID)
			deleteSubnamespace(org2.GUID, space22.GUID)
			deleteSubnamespace(org2.GUID, space23.GUID)
			deleteSubnamespace(org3.GUID, space31.GUID)
			deleteSubnamespace(org3.GUID, space32.GUID)
			deleteSubnamespace(org3.GUID, space33.GUID)
			waitForNamespaceDeletion(org1.GUID, space11.GUID)
			waitForNamespaceDeletion(org1.GUID, space12.GUID)
			waitForNamespaceDeletion(org1.GUID, space13.GUID)
			waitForNamespaceDeletion(org2.GUID, space21.GUID)
			waitForNamespaceDeletion(org2.GUID, space22.GUID)
			waitForNamespaceDeletion(org2.GUID, space23.GUID)
			waitForNamespaceDeletion(org3.GUID, space31.GUID)
			waitForNamespaceDeletion(org3.GUID, space32.GUID)
			waitForNamespaceDeletion(org3.GUID, space33.GUID)
			deleteSubnamespace(rootNamespace, org1.GUID)
			deleteSubnamespace(rootNamespace, org2.GUID)
			deleteSubnamespace(rootNamespace, org3.GUID)
		})

		It("lists the spaces the user has role in", func() {
			Eventually(getSpacesFn(tokenAuthHeader)).Should(SatisfyAll(
				HaveKeyWithValue("pagination", HaveKeyWithValue("total_results", BeNumerically(">=", 6))),
				HaveKeyWithValue("resources", ContainElements(
					HaveKeyWithValue("name", space11.Name),
					HaveKeyWithValue("name", space12.Name),
					HaveKeyWithValue("name", space21.Name),
					HaveKeyWithValue("name", space22.Name),
					HaveKeyWithValue("name", space31.Name),
					HaveKeyWithValue("name", space32.Name),
				))))
			Consistently(getSpacesFn(tokenAuthHeader), "5s").ShouldNot(
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
				_, err := getSpacesFn(tokenAuthHeader)()
				Expect(err).To(MatchError(ContainSubstring(strconv.Itoa(http.StatusUnauthorized))))
			})
		})

		When("filtering by organization GUIDs", func() {
			It("only lists spaces beloging to the orgs", func() {
				Eventually(getSpacesWithQueryFn(tokenAuthHeader, map[string]string{"organization_guids": fmt.Sprintf("%s,%s", org1.GUID, org3.GUID)})).Should(
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

func getSpacesFn(authHeaderValue string) func() (map[string]interface{}, error) {
	return getSpacesWithQueryFn(authHeaderValue, nil)
}

func getSpacesWithQueryFn(authHeaderValue string, query map[string]string) func() (map[string]interface{}, error) {
	return func() (map[string]interface{}, error) {
		spacesUrl, err := url.Parse(apiServerRoot)
		if err != nil {
			return nil, err
		}

		spacesUrl.Path = apis.SpacesEndpoint
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
}
