package smoke_test

import (
	"code.cloudfoundry.org/korifi/tests/helpers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("cf unbind-service", func() {
	var (
		serviceName   string
		unbindSession *Session
	)
	BeforeEach(func() {
		serviceName = uuid.NewString()
	})

	JustBeforeEach(func() {
		unbindSession = helpers.Cf("unbind-service", sharedData.BuildpackAppName, serviceName)
	})

	Describe("Unbinding from user-provided service instances", func() {
		BeforeEach(func() {
			Expect(
				helpers.Cf("create-user-provided-service", serviceName, "-p", `{"key1":"value1","key2":"value2"}`),
			).To(Exit(0))
			Expect(helpers.Cf("bind-service", sharedData.BuildpackAppName, serviceName)).To(Exit(0))
		})

		It("succeeds", func() {
			Expect(unbindSession).To(Exit(0))
		})
	})

	Describe("Uninding from managed service instances", func() {
		BeforeEach(func() {
			brokerName := uuid.NewString()
			Expect(helpers.Cf(
				"create-service-broker",
				brokerName,
				"broker-user",
				"broker-password",
				helpers.GetInClusterURL(getAppGUID(sharedData.BrokerAppName)),
			)).To(Exit(0))

			Expect(helpers.Cf("enable-service-access", "sample-service", "-b", brokerName)).To(Exit(0))
			session := helpers.Cf("create-service", "sample-service", "sample", serviceName, "-b", brokerName)
			Expect(session).To(Exit(0))
			Expect(helpers.Cf("bind-service", sharedData.BuildpackAppName, serviceName)).To(Exit(0))
		})

		It("succeeds", func() {
			Expect(unbindSession).To(Exit(0))
		})
	})
})
