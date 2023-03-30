package e2e_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/tests/e2e/helpers"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Processes", func() {
	var (
		spaceGUID   string
		appGUID     string
		processGUID string
		restyClient *helpers.CorrelatedRestyClient
		resp        *resty.Response
		errResp     cfErrs
	)

	BeforeEach(func() {
		restyClient = certClient
		errResp = cfErrs{}
		spaceGUID = createSpace(generateGUID("space"), commonTestOrgGUID)
		appGUID, _ = pushTestApp(spaceGUID, procfileAppBitsFile)
		processGUID = getProcess(appGUID, "web").GUID
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	Describe("List processes for app", func() {
		var (
			space2GUID     string
			app2GUID       string
			requestAppGUID string
			result         resourceList[resource]
		)

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().SetResult(&result).Get("/v3/apps/" + requestAppGUID + "/processes")
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user is authorized in the space", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", certUserName, spaceGUID)
				requestAppGUID = appGUID
			})

			It("returns the processes for the app", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

				Expect(processGUID).To(HavePrefix("cf-proc-"))
				Expect(processGUID).To(HaveSuffix("-web"))
				Expect(result.Resources).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(processGUID)}),
				))
			})
		})

		When("the user is NOT authorized in the space", func() {
			BeforeEach(func() {
				space2GUID = createSpace(generateGUID("space2"), commonTestOrgGUID)
				app2GUID, _ = pushTestApp(space2GUID, procfileAppBitsFile)
				requestAppGUID = app2GUID
			})

			AfterEach(func() {
				deleteSpace(space2GUID)
			})

			It("returns 404 NotFound", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusNotFound))
			})
		})
	})

	Describe("List sidecars", Ordered, func() {
		var list resourceList[resource]

		BeforeEach(func() {
			list = resourceList[resource]{}

			createSpaceRole("space_developer", certUserName, spaceGUID)
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

	Describe("Get process stats", func() {
		var processStats resourceList[statsResource]

		BeforeEach(func() {
			createSpaceRole("space_developer", certUserName, spaceGUID)
		})

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
		})

		When("we wait for the metrics to be ready", func() {
			BeforeEach(func() {
				Eventually(func(g Gomega) {
					var err error
					resp, err = restyClient.R().
						SetResult(&processStats).
						SetError(&errResp).
						Get("/v3/processes/" + processGUID + "/stats")
					g.Expect(err).NotTo(HaveOccurred())

					// no 'g.' here - we require all calls to return 200
					Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

					g.Expect(processStats.Resources).ToNot(BeEmpty())
					g.Expect(processStats.Resources[0].Usage).ToNot(BeZero())
				}).Should(Succeed())
			})

			It("succeeds", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

				Expect(processStats.Resources).To(HaveLen(1))
				Expect(processStats.Resources[0].Usage).To(MatchFields(IgnoreExtras, Fields{
					"Mem":  Not(BeNil()),
					"CPU":  Not(BeNil()),
					"Time": Not(BeNil()),
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

		BeforeEach(func() {
			createSpaceRole("space_developer", certUserName, spaceGUID)
		})

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

	Describe("Scale a process", func() {
		var result responseResource
		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetBody(scaleResource{Instances: 2}).
				SetError(&errResp).
				SetResult(&result).
				Post("/v3/processes/" + processGUID + "/actions/scale")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns not found for users with no role in the space", func() {
			expectNotFoundError(resp, errResp, "Process")
		})

		When("the user is a space manager", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", certUserName, spaceGUID)
			})

			It("returns forbidden", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusForbidden))
			})
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			It("succeeds, and returns the process", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.GUID).To(Equal(processGUID))
			})
		})
	})

	Describe("Patch a process", func() {
		var result responseResource

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetBody(commandResource{Command: "new command"}).
				SetError(&errResp).
				SetResult(&result).
				Patch("/v3/processes/" + processGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			It("returns success", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.GUID).To(Equal(processGUID))
			})
		})

		When("the user is a space manager", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", certUserName, spaceGUID)
			})

			It("returns forbidden", func() {
				expectForbiddenError(resp, errResp)
			})
		})

		When("the user has no role", func() {
			It("returns not found", func() {
				expectNotFoundError(resp, errResp, "Process")
			})
		})
	})

	Describe("Patch process metadata", func() {
		var result responseResource

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetBody(metadataResource{Metadata: &metadataPatch{
					Annotations: &map[string]string{"foo": "bar"},
				}}).
				SetError(&errResp).
				SetResult(&result).
				Patch("/v3/processes/" + processGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			It("successfully patches the annotations", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(string(resp.Body())).To(ContainSubstring(`"foo":"bar"`))
				Expect(result.GUID).To(Equal(processGUID))
			})
		})

		When("the user is a space manager", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", certUserName, spaceGUID)
			})

			It("returns forbidden", func() {
				expectForbiddenError(resp, errResp)
			})
		})

		When("the user has no role", func() {
			It("returns not found", func() {
				expectNotFoundError(resp, errResp, "Process")
			})
		})
	})
})
