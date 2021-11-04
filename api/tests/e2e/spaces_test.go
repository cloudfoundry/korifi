package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Spaces", func() {
	Describe("creating spaces", func() {
		var (
			org                   hierarchicalNamespace
			spaceName             string
			createStatusCode      int
			createResponseHeaders http.Header
			createResponseBody    map[string]interface{}
		)

		createSpace := func(spaceName, orgName string) (int, http.Header, map[string]interface{}) {
			spacesUrl := apiServerRoot + "/v3/spaces"

			body := fmt.Sprintf(`{
                "name": "%s",
                "relationships": {
                  "organization": {
                    "data": {
                      "guid": "%s"
                    }
                  }
                }
            }`, spaceName, orgName)
			req, err := http.NewRequest(http.MethodPost, spacesUrl, strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())

			response, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer response.Body.Close()

			responseMap := map[string]interface{}{}
			Expect(json.NewDecoder(response.Body).Decode(&responseMap)).To(Succeed())

			return response.StatusCode, response.Header, responseMap
		}

		BeforeEach(func() {
			spaceName = generateGUID("space")
			org = createHierarchicalNamespace(rootNamespace, generateGUID("org"), repositories.OrgNameLabel)
			waitForSubnamespaceAnchor(rootNamespace, org.guid)
		})

		AfterEach(func() {
			spaceGuid, ok := createResponseBody["guid"].(string)
			Expect(ok).To(BeTrue())
			deleteSubnamespace(org.guid, spaceGuid)
			waitForNamespaceDeletion(org.guid, spaceGuid)
		})

		JustBeforeEach(func() {
			createStatusCode, createResponseHeaders, createResponseBody = createSpace(spaceName, org.guid)
		})

		It("creates a space", func() {
			Expect(createStatusCode).To(Equal(http.StatusCreated))
			Expect(createResponseHeaders["Content-Type"]).To(ConsistOf("application/json"))
			Expect(createResponseBody["name"]).To(Equal(spaceName))

			nsName, ok := createResponseBody["guid"].(string)
			Expect(ok).To(BeTrue())
			Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: nsName}, &corev1.Namespace{})).To(Succeed())
		})

		When("the space name already exists", func() {
			JustBeforeEach(func() {
				Expect(createStatusCode).To(Equal(http.StatusCreated))
			})

			It("returns an unprocessable entity error", func() {
				dupRespCode, _, dupRespBody := createSpace(spaceName, org.guid)
				Expect(dupRespCode).To(Equal(http.StatusUnprocessableEntity))

				Expect(dupRespBody).To(HaveKeyWithValue("errors", BeAssignableToTypeOf([]interface{}{})))
				errs := dupRespBody["errors"].([]interface{})
				Expect(errs[0]).To(SatisfyAll(
					HaveKeyWithValue("code", BeNumerically("==", 10008)),
					HaveKeyWithValue("detail", MatchRegexp(fmt.Sprintf(`Space '%s' already exists.`, spaceName))),
					HaveKeyWithValue("title", Equal("CF-UnprocessableEntity")),
				))
			})
		})
	})

	Describe("listing spaces", func() {
		var orgs []hierarchicalNamespace

		BeforeEach(func() {
			orgs = []hierarchicalNamespace{}
			for i := 1; i <= 3; i++ {
				orgDetails := createHierarchicalNamespace(rootNamespace, generateGUID("org"+strconv.Itoa(i)), repositories.OrgNameLabel)
				waitForSubnamespaceAnchor(rootNamespace, orgDetails.guid)

				for j := 1; j <= 2; j++ {
					spaceDetails := createHierarchicalNamespace(orgDetails.guid, generateGUID("space"+strconv.Itoa(j)), repositories.SpaceNameLabel)
					waitForSubnamespaceAnchor(orgDetails.guid, spaceDetails.guid)
					orgDetails.children = append(orgDetails.children, spaceDetails)
				}

				orgs = append(orgs, orgDetails)
			}
		})

		AfterEach(func() {
			for _, org := range orgs {
				for _, space := range org.children {
					deleteSubnamespace(org.guid, space.guid)
					waitForNamespaceDeletion(org.guid, space.guid)
				}
				deleteSubnamespace(rootNamespace, org.guid)
			}
		})

		It("lists all the spaces", func() {
			Eventually(getSpacesFn(), "60s").Should(SatisfyAll(
				HaveKeyWithValue("pagination", HaveKeyWithValue("total_results", BeNumerically(">=", 6))),
				HaveKeyWithValue("resources", ContainElements(
					HaveKeyWithValue("name", orgs[0].children[0].label),
					HaveKeyWithValue("name", orgs[0].children[1].label),
					HaveKeyWithValue("name", orgs[1].children[0].label),
					HaveKeyWithValue("name", orgs[1].children[1].label),
					HaveKeyWithValue("name", orgs[2].children[0].label),
					HaveKeyWithValue("name", orgs[2].children[1].label),
				))))
		})

		When("filtering by organization GUIDs", func() {
			It("only lists spaces beloging to the orgs", func() {
				Eventually(getSpacesWithQueryFn(map[string]string{"organization_guids": fmt.Sprintf("%s,%s", orgs[0].guid, orgs[2].guid)}), "60s").Should(
					HaveKeyWithValue("resources", ConsistOf(
						HaveKeyWithValue("name", orgs[0].children[0].label),
						HaveKeyWithValue("name", orgs[0].children[1].label),
						HaveKeyWithValue("name", orgs[2].children[0].label),
						HaveKeyWithValue("name", orgs[2].children[1].label),
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
