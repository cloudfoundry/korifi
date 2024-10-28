package smoke_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("apps", func() {
	It("buildpack app is reachable via its route", func() {
		appResponseShould(sharedData.BuildpackAppName, "/", SatisfyAll(
			HaveHTTPStatus(http.StatusOK),
			HaveHTTPBody(ContainSubstring("Hi, I'm Dorifi!")),
		))
	})

	It("docker app is reachable via its route", func() {
		appResponseShould(sharedData.DockerAppName, "/", SatisfyAll(
			HaveHTTPStatus(http.StatusOK),
			HaveHTTPBody(ContainSubstring("Hi, I'm not Dora!")),
		))
	})

	It("broker app is reachable via its route", func() {
		appResponseShould(sharedData.BrokerAppName, "/", SatisfyAll(
			HaveHTTPStatus(http.StatusOK),
			HaveHTTPBody(ContainSubstring("Hi, I'm the sample broker!")),
		))
	})
})
