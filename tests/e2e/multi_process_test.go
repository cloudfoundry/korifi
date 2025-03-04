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
		appGUID, _ = pushTestApp(spaceGUID, multiProcessAppBitsFile)
		workerProcessGUID = getProcess(appGUID, "worker").GUID
		Expect(curlApp(appGUID)).To(ContainSubstring("Hi, I'm Dorifi (web)!"))
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	Describe("Scale a worker process", func() {
		var result responseResource

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetBody(scaleResource{Instances: 1}).
				SetError(&errResp).
				SetResult(&result).
				Post("/v3/processes/" + workerProcessGUID + "/actions/scale")
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds, and returns the worker process", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.GUID).To(Equal(workerProcessGUID))
		})
	})
})
