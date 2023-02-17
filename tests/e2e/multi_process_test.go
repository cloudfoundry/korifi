package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Multi Process", func() {
	var (
		spaceGUID         string
		appGUID           string
		workerProcessGUID string
		resp              *resty.Response
		errResp           cfErrs
	)

	BeforeEach(func() {
		errResp = cfErrs{}
		spaceGUID = createSpace(generateGUID("space"), commonTestOrgGUID)
		appGUID, _ = pushTestApp(spaceGUID, multiProcessBitsFile)
		workerProcessGUID = getProcess(appGUID, "worker").GUID
		body := curlApp(appGUID, "")
		Expect(body).To(ContainSubstring("hello-world from a multi-process-sample app!"))
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	Describe("Scale a worker process", func() {
		var result responseResource
		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetBody(scaleResource{Instances: 1}).
				SetError(&errResp).
				SetResult(&result).
				Post("/v3/processes/" + workerProcessGUID + "/actions/scale")
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

			It("succeeds, and returns the worker process", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.GUID).To(Equal(workerProcessGUID))
			})
		})
	})
})
