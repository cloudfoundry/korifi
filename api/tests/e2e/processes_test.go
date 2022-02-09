package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Processes", func() {
	var (
		orgGUID   string
		spaceGUID string

		appGUID     string
		processGUID string

		resp *resty.Response
	)

	BeforeEach(func() {
		orgGUID = createOrg(generateGUID("org"))
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)
		spaceGUID = createSpace(generateGUID("space"), orgGUID)
		createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
	})

	AfterEach(func() {
		deleteOrg(orgGUID)
	})

	BeforeEach(func() {
		appGUID = pushNodeApp(spaceGUID)
		processGUID = getProcess(appGUID, "web")
	})

	Describe("listing sidecars", Ordered, func() {
		var (
			list    resourceList
			listErr cfErrs
			client  *resty.Client
		)

		BeforeEach(func() {
			client = tokenClient
			list = resourceList{}
			listErr = cfErrs{}
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetResult(&list).
				SetError(&listErr).
				Get("/v3/processes/" + processGUID + "/sidecars")

			Expect(err).NotTo(HaveOccurred())
		})

		It("fails without space permissions", func() {
			Expect(resp.StatusCode()).To(Equal(http.StatusNotFound))
			Expect(listErr.Errors).To(HaveLen(1))
			Expect(listErr.Errors[0].Code).To(Equal(10010))
			Expect(listErr.Errors[0].Title).To(Equal("CF-ResourceNotFound"))
			Expect(listErr.Errors[0].Detail).To(Equal("Process not found"))
		})

		When("the user is authorized in the space", func() {
			BeforeEach(func() {
				client = certClient
			})

			It("lists the (empty list of) sidecars", func() {
				Expect(resp.StatusCode()).To(Equal(http.StatusOK), string(resp.Body()))
				Expect(list.Resources).To(BeEmpty())
			})
		})
	})

	Describe("Fetch a process", func() {
		var result resource

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().SetResult(&result).Get("/v3/processes/" + processGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("can fetch the process", func() {
			Expect(resp.StatusCode()).To(Equal(http.StatusOK))
			Expect(result.GUID).To(Equal(processGUID))
		})
	})
})
