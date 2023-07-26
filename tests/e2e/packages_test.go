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
		BeforeEach(func() {
			createSpaceRole("space_developer", certUserName, spaceGUID)
		})

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

		It("succeeds", func() {
			Expect(resultErr.Errors).To(BeEmpty())
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

	Describe("Update", func() {
		BeforeEach(func() {
			createSpaceRole("space_developer", certUserName, spaceGUID)
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

		It("succeeds", func() {
			Expect(resultErr.Errors).To(BeEmpty())
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.GUID).To(Equal(packageGUID))
		})
	})

	Describe("Upload", func() {
		BeforeEach(func() {
			createSpaceRole("space_developer", certUserName, spaceGUID)
			packageGUID = createPackage(appGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetFile("bits", defaultAppBitsFile).
				SetError(&resultErr).
				SetResult(&result).
				Post("/v3/packages/" + packageGUID + "/upload")
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
		})
	})

	Describe("Get", func() {
		var result resource

		BeforeEach(func() {
			createSpaceRole("space_developer", certUserName, spaceGUID)
			packageGUID = createPackage(appGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetResult(&result).
				Get("/v3/packages/" + packageGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fetches the package", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.GUID).To(Equal(packageGUID))
		})
	})

	Describe("List Droplets", func() {
		var buildGUID string
		var resultList resourceList[resource]

		BeforeEach(func() {
			createSpaceRole("space_manager", certUserName, spaceGUID)

			resultList = resourceList[resource]{}
			packageGUID = createPackage(appGUID)
			uploadTestApp(packageGUID, defaultAppBitsFile)
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

		It("lists the droplet", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(resultList.Resources).To(HaveLen(1))
			Expect(resultList.Resources[0].GUID).To(Equal(buildGUID))
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
