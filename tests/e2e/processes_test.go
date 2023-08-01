package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Processes", func() {
	var (
		spaceGUID      string
		appGUID        string
		webProcessGUID string
		resp           *resty.Response
		errResp        cfErrs
	)

	BeforeEach(func() {
		errResp = cfErrs{}
		spaceGUID = createSpace(generateGUID("space"), commonTestOrgGUID)
		appGUID, _ = pushTestApp(spaceGUID, defaultAppBitsFile)
		webProcessGUID = getProcess(appGUID, "web").GUID
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	Describe("List processes for app", func() {
		var result resourceList[resource]

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().SetResult(&result).Get("/v3/apps/" + appGUID + "/processes")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the processes for the app", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

			Expect(webProcessGUID).To(HavePrefix("cf-proc-"))
			Expect(webProcessGUID).To(HaveSuffix("-web"))
			// If DEFAULT_APP_BITS_PATH is set, then there may also be non-web processes.
			// To avoid failures in this case, we only test that the web process is included in the response.
			Expect(result.Resources).To(ContainElement(
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(webProcessGUID)}),
			))
		})
	})

	Describe("List sidecars", Ordered, func() {
		var list resourceList[resource]

		JustBeforeEach(func() {
			var err error
			list = resourceList[resource]{}
			resp, err = adminClient.R().
				SetResult(&list).
				SetError(&errResp).
				Get("/v3/processes/" + webProcessGUID + "/sidecars")

			Expect(err).NotTo(HaveOccurred())
		})

		It("lists the (empty list of) sidecars", func() {
			Expect(resp.StatusCode()).To(Equal(http.StatusOK), string(resp.Body()))
			Expect(list.Resources).To(BeEmpty())
		})
	})

	Describe("Get process stats", func() {
		var processStats resourceList[statsResource]

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&processStats).
				SetError(&errResp).
				Get("/v3/processes/" + webProcessGUID + "/stats")

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
					resp, err = adminClient.R().
						SetResult(&processStats).
						SetError(&errResp).
						Get("/v3/processes/" + webProcessGUID + "/stats")
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
	})

	Describe("Fetch a process", func() {
		var result resource

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&result).
				Get("/v3/processes/" + webProcessGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("can fetch the process", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.GUID).To(Equal(webProcessGUID))
		})
	})

	Describe("Scale a process", func() {
		var result responseResource

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetBody(scaleResource{Instances: 2}).
				SetError(&errResp).
				SetResult(&result).
				Post("/v3/processes/" + webProcessGUID + "/actions/scale")
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds, and returns the process", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.GUID).To(Equal(webProcessGUID))
		})
	})

	Describe("Patch a process", func() {
		var result responseResource

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetBody(commandResource{Command: "new command"}).
				SetError(&errResp).
				SetResult(&result).
				Patch("/v3/processes/" + webProcessGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns success", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.GUID).To(Equal(webProcessGUID))
		})
	})

	Describe("Patch process metadata", func() {
		var result responseResource

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetBody(metadataResource{Metadata: &metadataPatch{
					Annotations: &map[string]string{"foo": "bar"},
				}}).
				SetError(&errResp).
				SetResult(&result).
				Patch("/v3/processes/" + webProcessGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("successfully patches the annotations", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(string(resp.Body())).To(ContainSubstring(`"foo":"bar"`))
			Expect(result.GUID).To(Equal(webProcessGUID))
		})
	})
})
