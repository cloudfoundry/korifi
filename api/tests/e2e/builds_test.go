package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Builds", func() {
	var (
		orgGUID   string
		spaceGUID string
		appGUID   string
		pkgGUID   string
		resp      *resty.Response
		result    buildResource
	)

	BeforeEach(func() {
		orgGUID = createOrg(generateGUID("org"))
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)
		spaceGUID = createSpace(generateGUID("space1"), orgGUID)
		createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
		appGUID = createApp(spaceGUID, generateGUID("app"))
		pkgGUID = createPackage(appGUID)
	})

	AfterEach(func() {
		deleteOrg(orgGUID)
	})

	Describe("get", func() {
		var buildGUID string

		BeforeEach(func() {
			buildGUID = createBuild(pkgGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetResult(&result).
				Get("/v3/builds/" + buildGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the build", func() {
			Expect(resp.StatusCode()).To(Equal(http.StatusOK))
			Expect(result.GUID).To(Equal(buildGUID))
		})
	})

	Describe("create", func() {
		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetBody(buildResource{Package: resource{GUID: pkgGUID}}).
				SetResult(&result).
				Post("/v3/builds")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the build", func() {
			Expect(resp.StatusCode()).To(Equal(http.StatusCreated))
			Expect(result.Package.GUID).To(Equal(pkgGUID))
		})
	})
})
