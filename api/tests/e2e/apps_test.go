package e2e_test

import (
	"net/http"
	"sync"

	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
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
			apps, err := get("/v3/apps", certAuthHeader)
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

	Describe("Fetch App", func() {
		var app presenter.AppResponse

		BeforeEach(func() {
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1.GUID, adminAuthHeader)
			app = createApp(space1.GUID, generateGUID("app1"), adminAuthHeader)
		})

		It("can fetch the app", func() {
			appResponse, err := get("/v3/apps/"+app.GUID, certAuthHeader)
			Expect(err).NotTo(HaveOccurred())
			Expect(appResponse).To(SatisfyAll(
				HaveKeyWithValue("name", app.Name),
				HaveKeyWithValue("guid", app.GUID),
			))
		})
	})

	Describe("Droplets", func() {
		var (
			app      presenter.AppResponse
			pkg      presenter.PackageResponse
			build    presenter.BuildResponse
			response map[string]interface{}
			httpErr  error
		)

		BeforeEach(func() {
			app = createApp(space1.GUID, generateGUID("app"), adminAuthHeader)
			pkg = createPackage(app.GUID, adminAuthHeader)
			uploadNodeApp(pkg.GUID, adminAuthHeader)
			build = createBuild(pkg.GUID, adminAuthHeader)
			Eventually(func() error {
				_, err := get("/v3/droplets/"+build.GUID, adminAuthHeader)
				return err
			}).Should(Succeed())
		})

		Describe("get current droplet", func() {
			BeforeEach(func() {
				setCurrentDroplet(app.GUID, build.GUID, adminAuthHeader)
			})

			JustBeforeEach(func() {
				response, httpErr = get("/v3/apps/"+app.GUID+"/droplets/current", certAuthHeader)
			})

			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1.GUID, adminAuthHeader)
			})

			It("succeeds", func() {
				Expect(httpErr).NotTo(HaveOccurred())
				Expect(response).To(HaveKeyWithValue("state", "STAGED"))
			})
		})

		Describe("set current droplet", func() {
			JustBeforeEach(func() {
				response, httpErr = patch("/v3/apps/"+app.GUID+"/relationships/current_droplet", certAuthHeader, payloads.AppSetCurrentDroplet{
					Relationship: payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: build.GUID,
						},
					},
				})
			})

			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1.GUID, adminAuthHeader)
			})

			It("returns 200", func() {
				Expect(httpErr).NotTo(HaveOccurred())
				Expect(response).To(HaveKeyWithValue("data", HaveKeyWithValue("guid", build.GUID)))
			})
		})
	})
})
