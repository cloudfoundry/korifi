package e2e_test

import (
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Apps", func() {
	var (
		space1GUID string
		appGUID    string
		resp       *resty.Response
	)

	BeforeEach(func() {
		space1GUID = createSpace(generateGUID("space1"), commonTestOrgGUID)
	})

	AfterEach(func() {
		deleteSpace(space1GUID)
	})

	Describe("List apps", func() {
		var (
			space2GUID, space3GUID       string
			app1GUID, app2GUID, app3GUID string
			app4GUID, app5GUID, app6GUID string
			result                       resourceList
		)

		BeforeEach(func() {
			space2GUID = createSpace(generateGUID("space2"), commonTestOrgGUID)
			space3GUID = createSpace(generateGUID("space3"), commonTestOrgGUID)

			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1GUID)
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space3GUID)

			app1GUID = createApp(space1GUID, generateGUID("app1"))
			app2GUID = createApp(space1GUID, generateGUID("app2"))
			app3GUID = createApp(space2GUID, generateGUID("app3"))
			app4GUID = createApp(space2GUID, generateGUID("app4"))
			app5GUID = createApp(space3GUID, generateGUID("app5"))
			app6GUID = createApp(space3GUID, generateGUID("app6"))
		})

		AfterEach(func() {
			deleteSpace(space2GUID)
			deleteSpace(space3GUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().SetResult(&result).Get("/v3/apps")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns apps only in authorized spaces", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

			Expect(result.Resources).To(ContainElements(
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(app1GUID)}),
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(app2GUID)}),
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(app5GUID)}),
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(app6GUID)}),
			))

			Expect(result.Resources).ToNot(ContainElements(
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(app3GUID)}),
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(app4GUID)}),
			))
		})
	})

	Describe("Create an app", func() {
		var appName string

		BeforeEach(func() {
			appName = generateGUID("app")
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().SetBody(appResource{
				resource: resource{
					Name: appName,
					Relationships: relationships{
						"space": {
							Data: resource{
								GUID: space1GUID,
							},
						},
					},
				},
			}).Post("/v3/apps")
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user has space developer role in the space", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1GUID)
			})

			It("succeeds", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			})

			When("the app name already exists in the space", func() {
				BeforeEach(func() {
					createApp(space1GUID, appName)
				})

				It("returns an unprocessable entity error", func() {
					Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
					Expect(resp).To(HaveRestyBody(ContainSubstring("CF-UniquenessError")))
				})
			})
		})

		When("the user cannot create apps in the space", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", rbacv1.UserKind, certUserName, space1GUID)
			})

			It("fails", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusForbidden))
				Expect(resp).To(HaveRestyBody(ContainSubstring("CF-NotAuthorized")))
			})
		})
	})

	Describe("Fetch an app", func() {
		var result resource

		BeforeEach(func() {
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1GUID)
			appGUID = createApp(space1GUID, generateGUID("app1"))
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().SetResult(&result).Get("/v3/apps/" + appGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("can fetch the app", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.GUID).To(Equal(appGUID))
		})
	})

	Describe("List app processes", func() {
		var result resourceList

		BeforeEach(func() {
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1GUID)
			appGUID = createApp(space1GUID, generateGUID("app"))
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().SetResult(&result).Get("/v3/apps/" + appGUID + "/processes")
			Expect(err).NotTo(HaveOccurred())
		})

		It("successfully lists the empty set of processes", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(BeEmpty())
		})
	})

	Describe("Get app process by type", func() {
		var (
			result      resource
			processGUID string
		)

		BeforeEach(func() {
			appGUID = pushTestApp(space1GUID)
			processGUID = getProcess(appGUID, "web")
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().SetResult(&result).Get("/v3/apps/" + appGUID + "/processes/web")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns a not-found error to users with no space access", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusNotFound))
			Expect(resp).To(HaveRestyBody(ContainSubstring("App not found")))
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1GUID)
			})

			It("successfully returns the process", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.GUID).To(Equal(processGUID))
			})
		})
	})

	Describe("List app routes", func() {
		var result resourceList

		BeforeEach(func() {
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1GUID)
			appGUID = createApp(space1GUID, generateGUID("app"))
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().SetResult(&result).Get("/v3/apps/" + appGUID + "/routes")
			Expect(err).NotTo(HaveOccurred())
		})

		It("successfully lists the empty set of routes", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(BeEmpty())
		})
	})

	Describe("Built apps", func() {
		var (
			pkgGUID   string
			buildGUID string
			result    resource
		)

		BeforeEach(func() {
			appGUID = createApp(space1GUID, generateGUID("app"))
			pkgGUID = createPackage(appGUID)
			uploadTestApp(pkgGUID)
			buildGUID = createBuild(pkgGUID)
			waitForDroplet(buildGUID)

			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1GUID)
		})

		Describe("Get app current droplet", func() {
			BeforeEach(func() {
				setCurrentDroplet(appGUID, buildGUID)
			})

			JustBeforeEach(func() {
				var err error
				resp, err = certClient.R().SetResult(&result).Get("/v3/apps/" + appGUID + "/droplets/current")
				Expect(err).NotTo(HaveOccurred())
			})

			It("succeeds", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.GUID).To(Equal(buildGUID))
			})
		})

		Describe("Set app current droplet", func() {
			type currentDropletResource struct {
				Data resource `json:"data"`
			}

			var result currentDropletResource

			JustBeforeEach(func() {
				var err error
				var body currentDropletResource
				body.Data.GUID = buildGUID

				resp, err = certClient.R().
					SetBody(body).
					SetResult(&result).
					Patch("/v3/apps/" + appGUID + "/relationships/current_droplet")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 200", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Data.GUID).To(Equal(buildGUID))
			})
		})

		Describe("Start an app", func() {
			var result appResource

			BeforeEach(func() {
				setCurrentDroplet(appGUID, buildGUID)
			})

			JustBeforeEach(func() {
				var err error
				resp, err = certClient.R().SetResult(&result).Post("/v3/apps/" + appGUID + "/actions/start")
				Expect(err).NotTo(HaveOccurred())
			})

			It("succeeds", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.State).To(Equal("STARTED"))
			})

			It("has the VCAP_SERVICES env var set", func() {
				Expect(getEnv(appGUID)).To(HaveKeyWithValue("environment_variables", HaveKey("VCAP_SERVICES")))
			})
		})

		Describe("Restart an app", func() {
			var result appResource

			BeforeEach(func() {
				setCurrentDroplet(appGUID, buildGUID)
			})

			JustBeforeEach(func() {
				var err error
				resp, err = certClient.R().SetResult(&result).Post("/v3/apps/" + appGUID + "/actions/restart")
				Expect(err).NotTo(HaveOccurred())
			})

			It("succeeds", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.State).To(Equal("STARTED"))
			})

			When("app environment has been changed", func() {
				BeforeEach(func() {
					setEnv(appGUID, map[string]interface{}{
						"foo": "var",
					})
				})

				It("sets the new env var to the app environment", func() {
					Expect(getEnv(appGUID)).To(HaveKeyWithValue("environment_variables", HaveKeyWithValue("foo", "var")))
				})
			})
		})

		Describe("Stop an app", func() {
			var result appResource

			JustBeforeEach(func() {
				var err error
				resp, err = certClient.R().SetResult(&result).Post("/v3/apps/" + appGUID + "/actions/stop")
				Expect(err).NotTo(HaveOccurred())
			})

			It("succeeds", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.State).To(Equal("STOPPED"))
			})
		})
	})

	Describe("Running Apps", func() {
		var processGUID string

		BeforeEach(func() {
			appGUID = pushTestApp(space1GUID)
			processGUID = getProcess(appGUID, "web")
		})

		Describe("Scale a process", func() {
			var result responseResource
			var errResp cfErrs
			JustBeforeEach(func() {
				var err error
				resp, err = certClient.R().
					SetBody(scaleResource{Instances: 2}).
					SetError(&errResp).
					SetResult(&result).
					Post("/v3/apps/" + appGUID + "/processes/web/actions/scale")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns not found for users with no role in the space", func() {
				expectNotFoundError(resp, errResp, repositories.AppResourceType)
			})

			When("the user is a space manager", func() {
				BeforeEach(func() {
					createSpaceRole("space_manager", rbacv1.UserKind, certUserName, space1GUID)
				})

				It("returns forbidden", func() {
					Expect(resp).To(HaveRestyStatusCode(http.StatusForbidden))
				})
			})

			When("the user is a space developer", func() {
				BeforeEach(func() {
					createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space1GUID)
				})

				It("succeeds, and returns the process", func() {
					Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
					Expect(result.GUID).To(Equal(processGUID))
				})
			})
		})
	})
})
