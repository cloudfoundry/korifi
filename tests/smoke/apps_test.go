package smoke_test

import (
	"crypto/tls"
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
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

func appResponseShould(appName, requestPath string, matchExpectations types.GomegaMatcher) {
	var httpClient http.Client
	httpClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	Eventually(func(g Gomega) {
		resp, err := httpClient.Get(fmt.Sprintf("https://%s.%s%s", appName, appsDomain, requestPath))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(resp).To(matchExpectations)
	}).Should(Succeed())
}
