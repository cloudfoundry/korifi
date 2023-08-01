package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Builds", func() {
	var (
		spaceGUID string
		appGUID   string
		pkgGUID   string
		resp      *resty.Response
		result    buildResource
	)

	BeforeEach(func() {
		spaceGUID = createSpace(generateGUID("space1"), commonTestOrgGUID)
		appGUID = createApp(spaceGUID, generateGUID("app"))
		pkgGUID = createPackage(appGUID)
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	Describe("get", func() {
		var buildGUID string

		BeforeEach(func() {
			buildGUID = createBuild(pkgGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&result).
				Get("/v3/builds/" + buildGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the build", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.GUID).To(Equal(buildGUID))
		})
	})

	Describe("create", func() {
		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetBody(buildResource{Package: resource{GUID: pkgGUID}}).
				SetResult(&result).
				Post("/v3/builds")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the build", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(result.Package.GUID).To(Equal(pkgGUID))
		})
	})

	Describe("update", func() {
		var buildGUID string

		BeforeEach(func() {
			buildGUID = createBuild(pkgGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&result).
				Patch("/v3/builds/" + buildGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("throws an unprocessable entity error", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
			Expect(resp).To(HaveRestyBody(ContainSubstring("Labels and annotations are not supported for builds")))
		})
	})
})
