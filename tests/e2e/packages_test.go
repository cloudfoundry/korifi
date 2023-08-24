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
		var packageRequest packageResource

		BeforeEach(func() {
			packageRequest = packageResource{
				typedResource: typedResource{
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
				},
			}
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetBody(packageRequest).
				SetError(&resultErr).
				SetResult(&result).
				Post("/v3/packages")
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds", func() {
			Expect(resultErr.Errors).To(BeEmpty())
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(result.GUID).ToNot(BeEmpty())
			Expect(result.Type).To(Equal("bits"))
		})

		When("a new package is created for the same app", func() {
			BeforeEach(func() {
				var err error
				resp, err = adminClient.R().
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

		When("the package is of type docker", func() {
			BeforeEach(func() {
				packageRequest.Type = "docker"
				packageRequest.Data = &packageData{
					Image: "eirini/dorini",
				}
			})

			It("succeeds", func() {
				Expect(resultErr.Errors).To(BeEmpty())
				Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
				Expect(result.GUID).ToNot(BeEmpty())
				Expect(result.Type).To(Equal("docker"))
			})
		})
	})

	Describe("Update", func() {
		BeforeEach(func() {
			packageGUID = createPackage(appGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
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
			packageGUID = createPackage(appGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
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
			packageGUID = createPackage(appGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
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
			resp, err = adminClient.R().
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
			package1GUID string
			package2GUID string
		)

		BeforeEach(func() {
			space2GUID = createSpace(generateGUID("space2"), commonTestOrgGUID)

			package1GUID = createPackage(appGUID)
			app2GUID := createApp(space2GUID, generateGUID("app2"))
			package2GUID = createPackage(app2GUID)
		})

		AfterEach(func() {
			deleteSpace(space2GUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&result).
				Get("/v3/packages")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns packages", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(ContainElements(
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(package1GUID)}),
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(package2GUID)}),
			))
		})
	})
})
