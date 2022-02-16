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
		orgGUID     string
		spaceGUID   string
		appGUID     string
		processGUID string
		client      *resty.Client
		resp        *resty.Response
		errResp     cfErrs
	)

	BeforeEach(func() {
		client = certClient
		errResp = cfErrs{}
		orgGUID = createOrg(generateGUID("org"))
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)
		spaceGUID = createSpace(generateGUID("space"), orgGUID)
		appGUID = pushNodeApp(spaceGUID)
		processGUID = getProcess(appGUID, "web")

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
		var list resourceList

		BeforeEach(func() {
			list = resourceList{}
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetResult(&list).
				SetError(&errResp).
				Get("/v3/processes/" + processGUID + "/sidecars")

			Expect(err).NotTo(HaveOccurred())
		})

		It("lists the (empty list of) sidecars", func() {
			Expect(resp.StatusCode()).To(Equal(http.StatusOK), string(resp.Body()))
			Expect(list.Resources).To(BeEmpty())
		})

		When("the user is not authorized in the space", func() {
			BeforeEach(func() {
				client = tokenClient
			})

			It("returns a not found error", func() {
				Expect(resp.StatusCode()).To(Equal(http.StatusNotFound))
				Expect(errResp.Errors).To(ConsistOf(
					cfErr{
						Detail: "Process not found",
						Title:  "CF-ResourceNotFound",
						Code:   10010,
					},
				))
			})
		})
	})

	Describe("getting process stats", func() {
		var processStats statsResourceList

		BeforeEach(func() {
			appGUID = pushNodeApp(spaceGUID)
			processGUID = getProcess(appGUID, "web")
			processStats = statsResourceList{}
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetResult(&processStats).
				SetError(&errResp).
				Get("/v3/processes/" + processGUID + "/stats")

			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

			Expect(processStats.Resources).To(HaveLen(1))
			Expect(processStats.Resources[0].State).To(Equal("RUNNING"))
			Expect(processStats.Resources[0].Type).To(Equal("web"))
		})

		When("the user is not authorized in the space", func() {
			BeforeEach(func() {
				client = tokenClient
			})

			It("returns a not found error", func() {
				Expect(resp.StatusCode()).To(Equal(http.StatusNotFound))
				Expect(errResp.Errors).To(ConsistOf(
					cfErr{
						Detail: "Process not found",
						Title:  "CF-ResourceNotFound",
						Code:   10010,
					},
				))
			})
		})
	})

	Describe("Fetch a process", func() {
		var result resource

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetResult(&result).
				Get("/v3/processes/" + processGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("can fetch the process", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.GUID).To(Equal(processGUID))
		})
	})
})
