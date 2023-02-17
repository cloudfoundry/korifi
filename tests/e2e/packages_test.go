package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Package", func() {
	var (
		spaceGUID   string
		appGUID     string
		packageGUID string
		resp        *resty.Response
		result      typedResource
		resultErr   cfErrs
	)

	BeforeEach(func() {
		spaceGUID = createSpace(generateGUID("space"), commonTestOrgGUID)
		appGUID = createApp(spaceGUID, generateGUID("app"))

		result = typedResource{}
		resultErr = cfErrs{}
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
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
					Metadata: &metadata{
						Labels: map[string]string{
							"foo": "bar",
						},
						Annotations: map[string]string{
							"foo.bar.com/baz": "18",
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
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			It("succeeds", func() {
				Expect(resultErr.Errors).To(HaveLen(0))
				Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
				Expect(result.GUID).ToNot(BeEmpty())
			})

			When("a new package is created for the same app", func() {
				BeforeEach(func() {
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
						SetResult(&result).
						Post("/v3/packages")
					Expect(err).NotTo(HaveOccurred())
					Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
				})

				It("succeeds", func() {
					Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
				})
			})
		})

		When("the user is a SpaceManager (i.e. can get apps but cannot create packages)", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", certUserName, spaceGUID)
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

	Describe("Update", func() {
		BeforeEach(func() {
			packageGUID = createPackage(appGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetBody(typedResource{
					Metadata: &metadata{
						Labels: map[string]string{
							"foo": "bar",
						},
						Annotations: map[string]string{
							"foo.bar.com/baz": "18",
						},
					},
				}).
				SetError(&resultErr).
				SetResult(&result).
				Patch("/v3/packages/" + packageGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails with a resource not found error", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusNotFound))
			Expect(resultErr.Errors).To(ConsistOf(cfErr{
				Detail: "Package not found. Ensure it exists and you have access to it.",
				Title:  "CF-ResourceNotFound",
				Code:   10010,
			}))
		})

		When("the user is a SpaceDeveloper", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			It("succeeds", func() {
				Expect(resultErr.Errors).To(HaveLen(0))
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.GUID).To(Equal(packageGUID))
			})
		})

		When("the user is a SpaceManager (i.e. can get the package but cannot update it)", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", certUserName, spaceGUID)
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
				SetFile("bits", procfileAppBitsFile).
				SetError(&resultErr).
				SetResult(&result).
				Post("/v3/packages/" + packageGUID + "/upload")
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user is a SpaceManager (i.e. can get apps but cannot update packages)", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", certUserName, spaceGUID)
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
				createSpaceRole("space_developer", certUserName, spaceGUID)
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
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			It("can fetch the package", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.GUID).To(Equal(packageGUID))
			})
		})
	})

	Describe("List Droplets", func() {
		var buildGUID string
		var resultList resourceList[resource]

		BeforeEach(func() {
			resultList = resourceList[resource]{}
			packageGUID = createPackage(appGUID)
			uploadTestApp(packageGUID, procfileAppBitsFile)
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
				createSpaceRole("space_manager", certUserName, spaceGUID)
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
			result       resourceList[resource]
			space2GUID   string
			space3GUID   string
			package1GUID string
			package2GUID string
			package3GUID string
		)

		BeforeEach(func() {
			space2GUID = createSpace(generateGUID("space2"), commonTestOrgGUID)
			space3GUID = createSpace(generateGUID("space3"), commonTestOrgGUID)

			createSpaceRole("space_developer", certUserName, spaceGUID)
			createSpaceRole("space_developer", certUserName, space2GUID)

			package1GUID = createPackage(appGUID)
			app2GUID := createApp(space2GUID, generateGUID("app2"))
			package2GUID = createPackage(app2GUID)
			app3GUID := createApp(space3GUID, generateGUID("app3"))
			package3GUID = createPackage(app3GUID)
		})

		AfterEach(func() {
			deleteSpace(space2GUID)
			deleteSpace(space3GUID)
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
