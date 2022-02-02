package e2e_test

import (
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Apps", func() {
	var (
		org      presenter.OrgResponse
		space1   presenter.SpaceResponse
		app      presenter.AppResponse
		result   map[string]interface{}
		httpResp *resty.Response
		httpErr  error
	)

	BeforeEach(func() {
		org = createOrg(generateGUID("org"))
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, org.GUID, adminAuthHeader)
		space1 = createSpace(generateGUID("space1"), org.GUID)
	})

	AfterEach(func() {
		deleteSubnamespace(rootNamespace, org.GUID)
	})

	Describe("List apps", func() {
		var (
			space2, space3                     presenter.SpaceResponse
			app1, app2, app3, app4, app5, app6 presenter.AppResponse
		)

		BeforeEach(func() {
			space2 = createSpace(generateGUID("space2"), org.GUID)
			space3 = createSpace(generateGUID("space3"), org.GUID)

			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1.GUID, adminAuthHeader)
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space3.GUID, adminAuthHeader)

			app1 = createApp(space1.GUID, generateGUID("app1"))
			app2 = createApp(space1.GUID, generateGUID("app2"))
			app3 = createApp(space2.GUID, generateGUID("app3"))
			app4 = createApp(space2.GUID, generateGUID("app4"))
			app5 = createApp(space3.GUID, generateGUID("app5"))
			app6 = createApp(space3.GUID, generateGUID("app6"))
		})

		JustBeforeEach(func() {
			httpResp, httpErr = certClient.R().SetResult(&result).Get("/v3/apps")
		})

		It("returns apps only in authorized spaces", func() {
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(httpResp.StatusCode()).To(Equal(http.StatusOK))

			Expect(result).To(
				HaveKeyWithValue("pagination", HaveKeyWithValue("total_results", BeNumerically(">=", 4))),
				HaveKeyWithValue("resources", ContainElements(
					HaveKeyWithValue("name", app1.Name),
					HaveKeyWithValue("name", app2.Name),
					HaveKeyWithValue("name", app5.Name),
					HaveKeyWithValue("name", app6.Name),
				)),
			)

			Expect(result).ToNot(
				HaveKeyWithValue("resources", ContainElements(
					HaveKeyWithValue("name", app3.Name),
					HaveKeyWithValue("name", app4.Name),
				)))
		})
	})

	Describe("Create an app", func() {
		JustBeforeEach(func() {
			httpResp, httpErr = certClient.R().SetBody(map[string]interface{}{
				"name": generateGUID("app"),
				"relationships": map[string]interface{}{
					"space": map[string]interface{}{
						"data": map[string]interface{}{
							"guid": space1.GUID,
						},
					},
				},
			}).Post("/v3/apps")
		})

		It("fails", func() {
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(httpResp.StatusCode()).To(Equal(http.StatusForbidden))
			Expect(httpResp.Body()).To(ContainSubstring("CF-NotAuthorized"))
		})

		When("the user has space developer role in the space", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1.GUID, adminAuthHeader)
			})

			It("succeeds", func() {
				Expect(httpErr).NotTo(HaveOccurred())
				Expect(httpResp.StatusCode()).To(Equal(http.StatusCreated))
			})
		})
	})

	Describe("Fetch an app", func() {
		BeforeEach(func() {
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1.GUID, adminAuthHeader)
			app = createApp(space1.GUID, generateGUID("app1"))
		})

		JustBeforeEach(func() {
			httpResp, httpErr = certClient.R().SetResult(&result).Get("/v3/apps/" + app.GUID)
		})

		It("can fetch the app", func() {
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(httpResp.StatusCode()).To(Equal(http.StatusOK))
			Expect(result).To(SatisfyAll(
				HaveKeyWithValue("name", app.Name),
				HaveKeyWithValue("guid", app.GUID),
			))
		})
	})

	Describe("List app processes", func() {
		BeforeEach(func() {
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1.GUID, adminAuthHeader)
			app = createApp(space1.GUID, generateGUID("app"))
		})

		JustBeforeEach(func() {
			httpResp, httpErr = certClient.R().SetResult(&result).Get("/v3/apps/" + app.GUID + "/processes")
		})

		It("successfully lists the empty set of processes", func() {
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(httpResp.StatusCode()).To(Equal(http.StatusOK))
			Expect(result).To(HaveKeyWithValue("resources", BeEmpty()))
		})
	})

	Describe("List app routes", func() {
		BeforeEach(func() {
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1.GUID, adminAuthHeader)
			app = createApp(space1.GUID, generateGUID("app"))
		})

		JustBeforeEach(func() {
			httpResp, httpErr = certClient.R().SetResult(&result).Get("/v3/apps/" + app.GUID + "/routes")
		})

		It("successfully lists the empty set of processes", func() {
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(httpResp.StatusCode()).To(Equal(http.StatusOK))
			Expect(result).To(HaveKeyWithValue("resources", BeEmpty()))
		})
	})

	Describe("Built apps", func() {
		var (
			pkg   presenter.PackageResponse
			build presenter.BuildResponse
		)

		BeforeEach(func() {
			app = createApp(space1.GUID, generateGUID("app"))
			pkg = createPackage(app.GUID, adminAuthHeader)
			uploadNodeApp(pkg.GUID, adminAuthHeader)
			build = createBuild(pkg.GUID, adminAuthHeader)

			Eventually(func() (int, error) {
				resp, err := adminClient.R().Get("/v3/droplets/" + build.GUID)
				return resp.StatusCode(), err
			}).Should(Equal(http.StatusOK))

			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1.GUID, adminAuthHeader)
		})

		Describe("Get app current droplet", func() {
			BeforeEach(func() {
				setCurrentDroplet(app.GUID, build.GUID, adminAuthHeader)
			})

			JustBeforeEach(func() {
				httpResp, httpErr = certClient.R().SetResult(&result).Get("/v3/apps/" + app.GUID + "/droplets/current")
			})

			It("succeeds", func() {
				Expect(httpErr).NotTo(HaveOccurred())
				Expect(httpResp.StatusCode()).To(Equal(http.StatusOK))
				Expect(result).To(HaveKeyWithValue("state", "STAGED"))
			})
		})

		Describe("Set app current droplet", func() {
			JustBeforeEach(func() {
				httpResp, httpErr = certClient.R().
					SetBody(map[string]interface{}{
						"data": map[string]interface{}{
							"guid": build.GUID,
						},
					}).
					SetResult(&result).
					Patch("/v3/apps/" + app.GUID + "/relationships/current_droplet")
			})

			It("returns 200", func() {
				Expect(httpErr).NotTo(HaveOccurred())
				Expect(httpResp.StatusCode()).To(Equal(http.StatusOK))
				Expect(result).To(HaveKeyWithValue("data", HaveKeyWithValue("guid", build.GUID)))
			})
		})

		Describe("Restart an app", func() {
			BeforeEach(func() {
				setCurrentDroplet(app.GUID, build.GUID, adminAuthHeader)
			})

			JustBeforeEach(func() {
				httpResp, httpErr = certClient.R().SetResult(&result).Post("/v3/apps/" + app.GUID + "/actions/restart")
			})

			It("succeeds", func() {
				Expect(httpErr).NotTo(HaveOccurred())
				Expect(httpResp.StatusCode()).To(Equal(http.StatusOK))
				Expect(result).To(HaveKeyWithValue("state", "STARTED"))
			})
		})
	})
})
