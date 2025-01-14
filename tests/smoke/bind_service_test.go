package smoke_test

import (
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/broker"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("cf bind-service", func() {
	var (
		appName     string
		serviceName string
		bindSession *Session
	)

	BeforeEach(func() {
		appName = uuid.NewString()
		Expect(helpers.Cf("create-app", appName)).To(Exit(0))

		serviceName = uuid.NewString()
	})

	JustBeforeEach(func() {
		bindSession = helpers.Cf("bind-service", appName, serviceName)
	})

	Describe("Binding to user-provided service instances", func() {
		BeforeEach(func() {
			Expect(
				helpers.Cf("create-user-provided-service", serviceName, "-p", `{"key1":"value1","key2":"value2"}`),
			).To(Exit(0))
		})

		It("binds the service to the app", func() {
			Expect(bindSession).To(Exit(0))
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
				sharedData.BrokerURL,
			)).To(Exit(0))
			DeferCleanup(func() {
				broker.NewDeleter(sharedData.RootNamespace).ForBrokerName(brokerName).Delete()
			})

			Expect(helpers.Cf("enable-service-access", "sample-service", "-b", brokerName)).To(Exit(0))
			session := helpers.Cf("create-service", "sample-service", "sample", serviceName, "-b", brokerName)
			Expect(session).To(Exit(0))
		})

		It("binds the service to the app", func() {
			Expect(bindSession).To(Exit(0))
		})
	})
})
