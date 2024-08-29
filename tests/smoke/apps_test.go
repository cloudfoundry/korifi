package smoke_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("apps", func() {
	It("buildpack app is reachable via its route", func() {
		appResponseShould(buildpackAppName, "/", SatisfyAll(
			HaveHTTPStatus(http.StatusOK),
			HaveHTTPBody(ContainSubstring("Hi, I'm Dorifi!")),
		))
	})

	It("docker app is reachable via its route", func() {
		appResponseShould(dockerAppName, "/", SatisfyAll(
			HaveHTTPStatus(http.StatusOK),
			HaveHTTPBody(ContainSubstring("Hi, I'm not Dora!")),
		))
	})

	It("broker app is reachable via its route", func() {
		appResponseShould(brokerAppName, "/", SatisfyAll(
			HaveHTTPStatus(http.StatusOK),
			HaveHTTPBody(ContainSubstring("Hi, I'm the sample broker!")),
		))
	})
})
