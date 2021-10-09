//go:build e2e
// +build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"code.cloudfoundry.org/cf-k8s-api/repositories"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("listing spaces", func() {
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
