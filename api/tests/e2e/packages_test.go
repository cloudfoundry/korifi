package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Package", func() {
	var (
		orgGUID     string
		spaceGUID   string
		appGUID     string
		packageGUID string
		resp        *resty.Response
		result      typedResource
		resultErr   cfErrs
	)

	BeforeEach(func() {
		orgGUID = createOrg(generateGUID("org"))
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)

		spaceGUID = createSpace(generateGUID("space"), orgGUID)
		appGUID = createApp(spaceGUID, generateGUID("app"))

		result = typedResource{}
		resultErr = cfErrs{}
	})

	AfterEach(func() {
		deleteOrg(orgGUID)
	})

	Describe("Create", func() {
		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetBody(typedResource{
					Type: "bits",
					resource: resource{
						Relationships: relationships{
							"app": relationship{Data: resource{GUID: appGUID}},
						},
					},
				}).
				SetError(&resultErr).
				SetResult(&result).
				Post("/v3/packages")
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails with a resource not found error", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
			Expect(resultErr.Errors).To(ConsistOf(cfErr{
				Detail: "App is invalid. Ensure it exists and you have access to it.",
				Title:  "CF-UnprocessableEntity",
				Code:   10008,
			}))
		})

		When("the user is a SpaceDeveloper", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("succeeds", func() {
				Expect(resultErr.Errors).To(HaveLen(0))
				Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
				Expect(result.GUID).ToNot(BeEmpty())
			})
		})

		When("the user is a SpaceManager (i.e. can get apps but cannot create packages)", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("fails with a forbidden error", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusForbidden))
				Expect(resultErr.Errors).To(ConsistOf(cfErr{
					Detail: "You are not authorized to perform the requested action",
					Title:  "CF-NotAuthorized",
					Code:   10003,
				}))
			})
		})
	})

	Describe("Upload", func() {
		BeforeEach(func() {
			packageGUID = createPackage(appGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetFile("bits", "assets/procfile.zip").
				SetError(&resultErr).
				SetResult(&result).
				Post("/v3/packages/" + packageGUID + "/upload")
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user is a SpaceManager (i.e. can get apps but cannot update packages)", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("fails with a forbidden error", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusForbidden))
				Expect(resultErr.Errors).To(ConsistOf(cfErr{
					Detail: "You are not authorized to perform the requested action",
					Title:  "CF-NotAuthorized",
					Code:   10003,
				}))
			})
		})

		When("the user is a SpaceDeveloper", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("succeeds", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			})
		})
	})

	Describe("Get", func() {
		var result resource

		BeforeEach(func() {
			packageGUID = createPackage(appGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetResult(&result).
				Get("/v3/packages/" + packageGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns a not-found error to users with no space access", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusNotFound))
			Expect(resp).To(HaveRestyBody(ContainSubstring("Package not found")))
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("can fetch the package", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.GUID).To(Equal(packageGUID))
			})
		})
	})

	Describe("List Droplets", func() {
		var buildGUID string
		var resultList resourceList

		BeforeEach(func() {
			resultList = resourceList{}
			packageGUID = createPackage(appGUID)
			uploadTestApp(packageGUID)
			buildGUID = createBuild(packageGUID)

			Eventually(func() (*resty.Response, error) {
				return adminClient.R().Get("/v3/droplets/" + buildGUID)
			}).Should(HaveRestyStatusCode(http.StatusOK))
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetResult(&resultList).
				Get("/v3/packages/" + packageGUID + "/droplets")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns not found for unauthorized users", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusNotFound))
			Expect(resp).To(HaveRestyBody(ContainSubstring("Package not found")))
		})

		When("the user is a space manager", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("lists the droplet", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(resultList.Resources).To(HaveLen(1))
				Expect(resultList.Resources[0].GUID).To(Equal(buildGUID))
			})
		})
	})

	Describe("List", func() {
		var (
			result       resourceList
			package1GUID string
			package2GUID string
			package3GUID string
		)

		BeforeEach(func() {
			space2GUID := createSpace(generateGUID("space2"), orgGUID)
			space3GUID := createSpace(generateGUID("space3"), orgGUID)

			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space2GUID)

			package1GUID = createPackage(appGUID)
			app2GUID := createApp(space2GUID, generateGUID("app2"))
			package2GUID = createPackage(app2GUID)
			app3GUID := createApp(space3GUID, generateGUID("app3"))
			package3GUID = createPackage(app3GUID)
		})

		When("the user has no space access", func() {
			JustBeforeEach(func() {
				var err error
				resp, err = tokenClient.R().
					SetResult(&result).
					Get("/v3/packages")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns a not-found error to users with no space access", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Resources).To(BeEmpty())
			})
		})

		When("the user is a space developer", func() {
			JustBeforeEach(func() {
				var err error
				resp, err = certClient.R().
					SetResult(&result).
					Get("/v3/packages")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns only the Packages in spaces where the user has access", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Resources).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(package1GUID)}),
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(package2GUID)}),
				))
				Expect(result.Resources).NotTo(ContainElement(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(package3GUID)}),
				))
			})
		})
	})
})
