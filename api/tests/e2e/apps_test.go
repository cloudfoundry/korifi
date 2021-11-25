package e2e_test

import (
	"encoding/json"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"github.com/go-http-utils/headers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Listing Apps", func() {
	var (
		org                                presenter.OrgResponse
		space1, space2, space3             presenter.SpaceResponse
		app1, app2, app3, app4, app5, app6 presenter.AppResponse
	)

	BeforeEach(func() {
		org = createOrg(uuid.NewString(), tokenAuthHeader)
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, org.GUID, tokenAuthHeader)

		space1 = createSpace(uuid.NewString(), org.GUID, tokenAuthHeader)
		space2 = createSpace(uuid.NewString(), org.GUID, tokenAuthHeader)
		space3 = createSpace(uuid.NewString(), org.GUID, tokenAuthHeader)

		createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1.GUID, tokenAuthHeader)
		createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space3.GUID, tokenAuthHeader)

		app1 = createApp(space1.GUID, uuid.NewString(), tokenAuthHeader)
		app2 = createApp(space1.GUID, uuid.NewString(), tokenAuthHeader)
		app3 = createApp(space2.GUID, uuid.NewString(), tokenAuthHeader)
		app4 = createApp(space2.GUID, uuid.NewString(), tokenAuthHeader)
		app5 = createApp(space3.GUID, uuid.NewString(), tokenAuthHeader)
		app6 = createApp(space3.GUID, uuid.NewString(), tokenAuthHeader)

		_, _ = app3, app4
	})

	AfterEach(func() {
		deleteSubnamespace(org.GUID, space1.GUID)
		deleteSubnamespace(org.GUID, space2.GUID)
		deleteSubnamespace(org.GUID, space3.GUID)
		deleteSubnamespace(rootNamespace, org.GUID)
	})

	It("returns apps only in authorized spaces", func() {
		apps := listApps(certAuthHeader)
		appNames := []string{}

		for _, app := range apps.Resources {
			appNames = append(appNames, app.Name)
		}

		Expect(appNames).To(ConsistOf(app1.Name, app2.Name, app5.Name, app6.Name))
	})
})

func listApps(authHeader string) presenter.AppListResponse {
	appsURL := apiServerRoot + apis.AppListEndpoint

	req, err := http.NewRequest(http.MethodGet, appsURL, nil)
	Expect(err).NotTo(HaveOccurred())

	req.Header.Add(headers.Authorization, authHeader)

	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()

	Expect(resp).To(HaveHTTPStatus(http.StatusOK))

	apps := presenter.AppListResponse{}
	err = json.NewDecoder(resp.Body).Decode(&apps)
	Expect(err).NotTo(HaveOccurred())

	return apps
}
