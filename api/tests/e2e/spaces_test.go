package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Spaces", func() {
	Describe("creating spaces", func() {
		var (
			org       presenter.OrgResponse
			spaceName string
			space     presenter.SpaceResponse
		)

		BeforeEach(func() {
			spaceName = generateGUID("space")
			org = createOrg(generateGUID("org"), tokenAuthHeader)
		})

		AfterEach(func() {
			deleteSubnamespace(org.GUID, space.GUID)
			waitForNamespaceDeletion(org.GUID, space.GUID)
			deleteSubnamespace(rootNamespace, org.GUID)
		})

		JustBeforeEach(func() {
			space = createSpace(spaceName, org.GUID)
		})

		It("creates a space", func() {
			Expect(space.Name).To(Equal(spaceName))

			Eventually(func() error {
				return k8sClient.Get(context.Background(), client.ObjectKey{Name: space.GUID}, &corev1.Namespace{})
			}).Should(Succeed())
		})

		When("the space name already exists", func() {
			It("returns an unprocessable entity error", func() {
				resp, err := createSpaceRaw(spaceName, org.GUID)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
				defer resp.Body.Close()

				bodyMap := map[string]interface{}{}
				err = json.NewDecoder(resp.Body).Decode(&bodyMap)
				Expect(err).NotTo(HaveOccurred())

				Expect(bodyMap).To(HaveKeyWithValue("errors", BeAssignableToTypeOf([]interface{}{})))
				errs := bodyMap["errors"].([]interface{})
				Expect(errs[0]).To(SatisfyAll(
					HaveKeyWithValue("code", BeNumerically("==", 10008)),
					HaveKeyWithValue("detail", MatchRegexp(fmt.Sprintf(`Space '%s' already exists.`, spaceName))),
					HaveKeyWithValue("title", Equal("CF-UnprocessableEntity")),
				))
			})
		})
	})

	Describe("listing spaces", func() {
		var (
			org1, org2, org3                                     presenter.OrgResponse
			space11, space12, space21, space22, space31, space32 presenter.SpaceResponse
		)

		BeforeEach(func() {
			org1 = createOrg(generateGUID("org1"), tokenAuthHeader)
			org2 = createOrg(generateGUID("org2"), tokenAuthHeader)
			org3 = createOrg(generateGUID("org3"), tokenAuthHeader)

			space11 = createSpace(generateGUID("space1"), org1.GUID)
			space12 = createSpace(generateGUID("space2"), org1.GUID)
			space21 = createSpace(generateGUID("space1"), org2.GUID)
			space22 = createSpace(generateGUID("space2"), org2.GUID)
			space31 = createSpace(generateGUID("space1"), org3.GUID)
			space32 = createSpace(generateGUID("space2"), org3.GUID)
		})

		AfterEach(func() {
			deleteSubnamespace(org1.GUID, space11.GUID)
			deleteSubnamespace(org1.GUID, space12.GUID)
			deleteSubnamespace(org2.GUID, space21.GUID)
			deleteSubnamespace(org2.GUID, space22.GUID)
			deleteSubnamespace(org3.GUID, space31.GUID)
			deleteSubnamespace(org3.GUID, space32.GUID)
			waitForNamespaceDeletion(org1.GUID, space11.GUID)
			waitForNamespaceDeletion(org1.GUID, space12.GUID)
			waitForNamespaceDeletion(org2.GUID, space21.GUID)
			waitForNamespaceDeletion(org2.GUID, space22.GUID)
			waitForNamespaceDeletion(org3.GUID, space31.GUID)
			waitForNamespaceDeletion(org3.GUID, space32.GUID)
			deleteSubnamespace(rootNamespace, org1.GUID)
			deleteSubnamespace(rootNamespace, org2.GUID)
			deleteSubnamespace(rootNamespace, org3.GUID)
		})

		It("lists all the spaces", func() {
			Eventually(getSpacesFn()).Should(SatisfyAll(
				HaveKeyWithValue("pagination", HaveKeyWithValue("total_results", BeNumerically(">=", 6))),
				HaveKeyWithValue("resources", ContainElements(
					HaveKeyWithValue("name", space11.Name),
					HaveKeyWithValue("name", space12.Name),
					HaveKeyWithValue("name", space21.Name),
					HaveKeyWithValue("name", space22.Name),
					HaveKeyWithValue("name", space31.Name),
					HaveKeyWithValue("name", space32.Name),
				))))
		})

		When("filtering by organization GUIDs", func() {
			It("only lists spaces beloging to the orgs", func() {
				Eventually(getSpacesWithQueryFn(map[string]string{"organization_guids": fmt.Sprintf("%s,%s", org1.GUID, org3.GUID)})).Should(
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

func getSpacesFn() func() (map[string]interface{}, error) {
	return getSpacesWithQueryFn(nil)
}

func getSpacesWithQueryFn(query map[string]string) func() (map[string]interface{}, error) {
	return func() (map[string]interface{}, error) {
		spacesUrl, err := url.Parse(apiServerRoot)
		if err != nil {
			return nil, err
		}

		spacesUrl.Path = "/v3/spaces"
		values := url.Values{}
		for key, val := range query {
			values.Set(key, val)
		}
		spacesUrl.RawQuery = values.Encode()

		resp, err := http.Get(spacesUrl.String())
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
