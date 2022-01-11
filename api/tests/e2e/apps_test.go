package e2e_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

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
		org = createOrg(generateGUID("org"), tokenAuthHeader)
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, org.GUID, tokenAuthHeader)
		space1 = createSpace(generateGUID("space1"), org.GUID, tokenAuthHeader)
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
			space2 = createSpace(generateGUID("space2"), org.GUID, tokenAuthHeader)
			space3 = createSpace(generateGUID("space3"), org.GUID, tokenAuthHeader)

			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1.GUID, tokenAuthHeader)
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space3.GUID, tokenAuthHeader)

			app1 = createApp(space1.GUID, generateGUID("app1"), tokenAuthHeader)
			app2 = createApp(space1.GUID, generateGUID("app2"), tokenAuthHeader)
			app3 = createApp(space2.GUID, generateGUID("app3"), tokenAuthHeader)
			app4 = createApp(space2.GUID, generateGUID("app4"), tokenAuthHeader)
			app5 = createApp(space3.GUID, generateGUID("app5"), tokenAuthHeader)
			app6 = createApp(space3.GUID, generateGUID("app6"), tokenAuthHeader)

			_, _ = app3, app4
		})

		AfterEach(func() {
			deleteSubnamespace(org.GUID, space2.GUID)
			deleteSubnamespace(org.GUID, space3.GUID)
		})

		It("returns apps only in authorized spaces", func() {
			Eventually(getAppsFn(certAuthHeader)).Should(SatisfyAll(
				HaveKeyWithValue("pagination", HaveKeyWithValue("total_results", BeNumerically(">=", 4))),
				HaveKeyWithValue("resources", ContainElements(
					HaveKeyWithValue("name", app1.Name),
					HaveKeyWithValue("name", app2.Name),
					HaveKeyWithValue("name", app5.Name),
					HaveKeyWithValue("name", app6.Name),
				))))
			Consistently(getAppsFn(certAuthHeader), "5s").ShouldNot(
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
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1.GUID, tokenAuthHeader)
			})

			It("succeeds", func() {
				Expect(createErr).NotTo(HaveOccurred())
				defer createResponse.Body.Close()
				Expect(createResponse).To(HaveHTTPStatus(http.StatusCreated))
			})
		})
	})
})

func getAppsFn(authHeaderValue string) func() (map[string]interface{}, error) {
	return func() (map[string]interface{}, error) {
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
}
