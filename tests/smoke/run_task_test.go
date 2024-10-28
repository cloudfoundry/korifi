package smoke_test

import (
	"code.cloudfoundry.org/korifi/tests/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("cf run-task", func() {
	It("succeeds", func() {
		Eventually(helpers.Cf("run-task", sharedData.BuildpackAppName, "-c", `echo "Hello from the task"`)).Should(Exit(0))
	})
})
