package e2e_test

import (
	"fmt"
	"net/http"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Processes", func() {
	var (
		orgGUID     string
		spaceGUID   string
		appGUID     string
		processGUID string
		restyClient *resty.Client
		resp        *resty.Response
		errResp     cfErrs
	)

	BeforeEach(func() {
		restyClient = certClient
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

	Describe("listing sidecars", Ordered, func() {
		var list resourceList

		BeforeEach(func() {
			list = resourceList{}
		})

		JustBeforeEach(func() {
			var err error
			resp, err = restyClient.R().
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
				restyClient = tokenClient
			})

			It("returns a not found error", func() {
				expectNotFoundError(resp, errResp, "Process")
			})
		})
	})

	Describe("getting process stats", func() {
		var processStats statsResourceList

		JustBeforeEach(func() {
			var err error
			resp, err = restyClient.R().
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

		When("we wait for the metrics to be ready", func() {
			BeforeEach(func() {
				// use an eventually to check when the pod metrics become available
				Eventually(func() presenter.ProcessUsage {
					var err error
					resp, err = restyClient.R().
						SetResult(&processStats).
						SetError(&errResp).
						Get("/v3/processes/" + processGUID + "/stats")
					Expect(err).NotTo(HaveOccurred())

					return processStats.Resources[0].Usage
				}, 60*time.Second).ShouldNot(Equal(presenter.ProcessUsage{}))
			})
			It("succeeds", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

				fmt.Printf("\nWAIT RESPONSE\n\n%#v\n\n\n", resp)
				fmt.Printf("\nWAIT PROCESS STATS\n\n%#v\n\n\n", processStats)
				Expect(processStats.Resources).To(HaveLen(1))
				Expect(processStats.Resources[0].Usage).To(MatchFields(IgnoreExtras, Fields{
					"Mem":  Not(BeNil()),
					"CPU":  Not(BeNil()),
					"Time": Not(BeNil()),
					//"Disk": Not(BeNil()), // Disk is currently empty
				}))
			})
		})

		When("the user is not authorized in the space", func() {
			BeforeEach(func() {
				restyClient = tokenClient
			})

			It("returns a not found error", func() {
				expectNotFoundError(resp, errResp, "Process")
			})
		})
	})

	Describe("Fetch a process", func() {
		var result resource

		JustBeforeEach(func() {
			var err error
			resp, err = restyClient.R().
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
