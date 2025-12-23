package smoke_test

import (
	"net/http"
	"os"

	"code.cloudfoundry.org/korifi/tests/helpers"
	"github.com/BooleanCat/go-functional/v2/it"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/onsi/gomega/gexec"
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

	When("getting apps manifest", func() {
		var (
			manifestFile *os.File
			err          error
		)
		BeforeEach(func() {
			manifestFile, err = os.CreateTemp("", "")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the app manifest", func() {
			session := helpers.Cf("create-app-manifest", sharedData.DockerAppName, "-p", manifestFile.Name())
			Expect(session).To(Exit(0))
			lines := it.MustCollect(it.LinesString(session.Out))
			Expect(lines).To(ContainElement(ContainSubstring("Manifest file created successfully")))
		})

		AfterEach(func() {
			Expect(os.RemoveAll(manifestFile.Name())).To(Succeed())
		})
	})
})
