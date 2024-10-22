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
	var (
		appName     string
		serviceName string
	)

	BeforeEach(func() {
		// Binding apps to service instances changes their VCAP_SERVICES to
		// reflect all the app bindings. Therefore, bind tests need dedicated
		// test app (i.e. cannot use one created by the suite)
		appName = uuid.NewString()
		Expect(helpers.Cf("push", appName, "-p", "../assets/dorifi")).To(Exit(0))
	})

	JustBeforeEach(func() {
		Expect(helpers.Cf("bind-service", appName, serviceName)).To(Exit(0))
		Expect(helpers.Cf("restart", appName)).To(Exit(0))
	})

	Describe("Binding to user-provided service instances", func() {
		BeforeEach(func() {
			serviceName = uuid.NewString()
			Expect(
				helpers.Cf("create-user-provided-service", serviceName, "-p", `{"key1":"value1","key2":"value2"}`),
			).To(Exit(0))
		})

		It("binds the service to the app", func() {
			appResponseShould(appName, "/env.json", SatisfyAll(
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

	Describe("Binding to managed service instances", func() {
		BeforeEach(func() {
			brokerName := uuid.NewString()
			Expect(helpers.Cf(
				"create-service-broker",
				brokerName,
				"broker-user",
				"broker-password",
				helpers.GetInClusterURL(getAppGUID(brokerAppName)),
			)).To(Exit(0))

			serviceName = uuid.NewString()
			session := helpers.Cf("create-service", "sample-service", "sample", serviceName, "-b", brokerName)
			Expect(session).To(Exit(0))
		})

		It("binds the service to the app", func() {
			appResponseShould(appName, "/env.json", SatisfyAll(
				HaveHTTPStatus(http.StatusOK),
				HaveHTTPBody(
					MatchJSONPath("$.VCAP_SERVICES", Not(BeEmpty())),
				),
			))
		})
	})
})
