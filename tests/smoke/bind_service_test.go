package smoke_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/tests/helpers"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("cf bind-service", func() {
	BeforeEach(func() {
		serviceName := uuid.NewString()

		Expect(
			helpers.Cf("create-user-provided-service", serviceName, "-p", `{"key1":"value1","key2":"value2"}`),
		).To(Exit(0))

		Expect(helpers.Cf("bind-service", buildpackAppName, serviceName)).To(Exit(0))
		Expect(helpers.Cf("restart", buildpackAppName)).To(Exit(0))
	})

	It("binds the service to the app", func() {
		appResponseShould(buildpackAppName, "/env.json", SatisfyAll(
			HaveHTTPStatus(http.StatusOK),
			HaveHTTPBody(
				MatchJSONPath("$.VCAP_SERVICES", SatisfyAll(
					MatchJSONPath(`$["user-provided"][0].credentials.key1`, "value1"),
					MatchJSONPath(`$["user-provided"][0].credentials.key2`, "value2"),
				)),
			),
		))
	})
})
