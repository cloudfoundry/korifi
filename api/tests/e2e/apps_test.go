package e2e_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Apps", func() {
	var (
		org    presenter.OrgResponse
		space1 presenter.SpaceResponse
	)

	BeforeEach(func() {
		org = createOrg(generateGUID("org"), adminAuthHeader)
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, org.GUID, adminAuthHeader)
		space1 = createSpace(generateGUID("space1"), org.GUID, adminAuthHeader)
	})

	AfterEach(func() {
		deleteSubnamespace(org.GUID, space1.GUID)
		deleteSubnamespace(rootNamespace, org.GUID)
	})

	Describe("List", func() {
		var (
			space2, space3                     presenter.SpaceResponse
			app1, app2, app3, app4, app5, app6 presenter.AppResponse
		)

		BeforeEach(func() {
			space2 = createSpace(generateGUID("space2"), org.GUID, adminAuthHeader)
			space3 = createSpace(generateGUID("space3"), org.GUID, adminAuthHeader)

			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1.GUID, adminAuthHeader)
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space3.GUID, adminAuthHeader)

			app1 = createApp(space1.GUID, generateGUID("app1"), adminAuthHeader)
			app2 = createApp(space1.GUID, generateGUID("app2"), adminAuthHeader)
			app3 = createApp(space2.GUID, generateGUID("app3"), adminAuthHeader)
			app4 = createApp(space2.GUID, generateGUID("app4"), adminAuthHeader)
			app5 = createApp(space3.GUID, generateGUID("app5"), adminAuthHeader)
			app6 = createApp(space3.GUID, generateGUID("app6"), adminAuthHeader)

			_, _ = app3, app4
		})

		AfterEach(func() {
			var wg sync.WaitGroup
			wg.Add(2)
			for _, id := range []string{space2.GUID, space3.GUID} {
				asyncDeleteSubnamespace(org.GUID, id, &wg)
			}
			wg.Wait()
		})

		It("returns apps only in authorized spaces", func() {
			apps, err := getApps(certAuthHeader)
			Expect(err).NotTo(HaveOccurred())
			Expect(apps).To(SatisfyAll(
				HaveKeyWithValue("pagination", HaveKeyWithValue("total_results", BeNumerically(">=", 4))),
				HaveKeyWithValue("resources", ContainElements(
					HaveKeyWithValue("name", app1.Name),
					HaveKeyWithValue("name", app2.Name),
					HaveKeyWithValue("name", app5.Name),
					HaveKeyWithValue("name", app6.Name),
				))))

			Expect(apps).ToNot(
				HaveKeyWithValue("resources", ContainElements(
					HaveKeyWithValue("name", app3.Name),
					HaveKeyWithValue("name", app4.Name),
				)))
		})
	})

	Describe("Create", func() {
		var (
			createResponse *http.Response
			createErr      error
		)

		JustBeforeEach(func() {
			appGUID := generateGUID("app")
			createResponse, createErr = createAppRaw(space1.GUID, appGUID, certAuthHeader)
		})

		It("fails", func() {
			Expect(createErr).NotTo(HaveOccurred())
			defer createResponse.Body.Close()
			Expect(createResponse).To(HaveHTTPStatus(http.StatusForbidden))
			Expect(createResponse).To(HaveHTTPBody(ContainSubstring("CF-NotAuthorized")))
		})

		When("the user has space developer role in the space", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1.GUID, adminAuthHeader)
			})

			It("succeeds", func() {
				Expect(createErr).NotTo(HaveOccurred())
				defer createResponse.Body.Close()
				Expect(createResponse).To(HaveHTTPStatus(http.StatusCreated))
			})
		})
	})
})

func getApps(authHeaderValue string) (map[string]interface{}, error) {
	appsURL, err := url.Parse(apiServerRoot)
	if err != nil {
		return nil, err
	}

	appsURL.Path = apis.AppListEndpoint

	resp, err := httpReq(http.MethodGet, appsURL.String(), authHeaderValue, nil)
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
