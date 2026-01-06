package smoke_test

import (
	"os"

	"code.cloudfoundry.org/korifi/tests/helpers"
	"github.com/BooleanCat/go-functional/v2/it"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("cf create-app-manifest", func() {
	var (
		manifestFile *os.File
		err          error
	)
	BeforeEach(func() {
		manifestFile, err = os.CreateTemp("", "")
		DeferCleanup(func() {
			Expect(os.RemoveAll(manifestFile.Name())).To(Succeed())
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns the app manifest", func() {
		session := helpers.Cf("create-app-manifest", sharedData.DockerAppName, "-p", manifestFile.Name())
		Expect(session).To(Exit(0))
		lines := it.MustCollect(it.LinesString(session.Out))
		Expect(lines).To(ContainElement(ContainSubstring("Manifest file created successfully")))
	})
})
