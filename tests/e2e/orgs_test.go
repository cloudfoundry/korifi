package e2e_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	. "github.com/onsi/gomega/gstruct"

	"code.cloudfoundry.org/cf-k8s-api/presenter"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Orgs", func() {
	Describe("creating orgs", func() {
		var orgName string

		BeforeEach(func() {
			orgName = generateGUID("org")
		})

		AfterEach(func() {
			deleteSubnamespaceByLabel(rootNamespace, orgName)
		})

		It("creates an org", func() {
			orgsUrl := apiServerRoot + "/v3/organizations"

			body := fmt.Sprintf(`{ "name": "%s" }`, orgName)
			req, err := http.NewRequest(http.MethodPost, orgsUrl, strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			Expect(resp.Header["Content-Type"]).To(ConsistOf("application/json"))
			responseMap := map[string]interface{}{}
			Expect(json.NewDecoder(resp.Body).Decode(&responseMap)).To(Succeed())
			Expect(responseMap["name"]).To(Equal(orgName))
		})
	})

	Describe("listing orgs", func() {
		var orgs []hierarchicalNamespace

		BeforeEach(func() {
			orgs = []hierarchicalNamespace{}
			for i := 1; i < 4; i++ {
				orgDetails := createHierarchicalNamespace(rootNamespace, generateGUID("org"+strconv.Itoa(i)), repositories.OrgNameLabel)
				orgs = append(orgs, orgDetails)
				waitForSubnamespaceAnchor(rootNamespace, orgDetails.guid)
			}
		})

		AfterEach(func() {
			for _, org := range orgs {
				deleteSubnamespace(rootNamespace, org.guid)
			}
		})

		It("returns all 3 orgs", func() {
			Eventually(getOrgsFn()).Should(ContainElements(
				MatchFields(IgnoreExtras, Fields{"Name": Equal(orgs[0].label)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(orgs[1].label)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(orgs[2].label)}),
			))
		})

		When("org names are filtered", func() {
			It("returns orgs 1 & 3", func() {
				Eventually(getOrgsFn(orgs[0].label, orgs[2].label)).Should(ContainElements(
					MatchFields(IgnoreExtras, Fields{"Name": Equal(orgs[0].label)}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal(orgs[2].label)}),
				))
				Consistently(getOrgsFn(orgs[0].label, orgs[2].label), "2s").ShouldNot(ContainElement(
					MatchFields(IgnoreExtras, Fields{"Name": Equal(orgs[1].label)}),
				))
			})
		})
	})
})

func getOrgsFn(names ...string) func() ([]presenter.OrgResponse, error) {
	return func() ([]presenter.OrgResponse, error) {
		orgsUrl := apiServerRoot + "/v3/organizations"

		if len(names) > 0 {
			orgsUrl += "?names=" + strings.Join(names, ",")
		}

		req, err := http.NewRequest(http.MethodGet, orgsUrl, nil)
		if err != nil {
			return nil, err
		}

		resp, err := http.DefaultClient.Do(req)
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

		orgList := presenter.OrgListResponse{}
		err = json.Unmarshal(bodyBytes, &orgList)
		if err != nil {
			return nil, err
		}

		return orgList.Resources, nil
	}
}
