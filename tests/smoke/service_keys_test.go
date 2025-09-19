package smoke_test

import (
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/broker"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("cf service-key", func() {
	var (
		serviceName        string
		serviceKeysSession *Session
		serviceKeyName     string
	)

	BeforeEach(func() {
		serviceKeyName = uuid.NewString()
		appName := uuid.NewString()
		Expect(helpers.Cf("create-app", appName)).To(Exit(0))

		serviceName = uuid.NewString()
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
		serviceKeysSession = helpers.Cf("create-service", "sample-service", "sample", serviceName, "-b", brokerName)
		Expect(serviceKeysSession).To(Exit(0))
	})

	JustBeforeEach(func() {
		serviceKeysSession = helpers.Cf("create-service-key", serviceName, serviceKeyName, "--wait")
		Expect(serviceKeysSession).To(Exit(0))
	})

	It("creates a service key", func() {
		serviceKeysSession = helpers.Cf("service-key", serviceName, serviceKeyName)
		Expect(serviceKeysSession).To(Exit(0))
		lines := it.MustCollect(it.LinesString(serviceKeysSession.Out))
		Expect(lines).To(ContainElements(
			ContainSubstring(`"credentials"`),
			ContainSubstring(`"username"`),
			ContainSubstring(`"password"`),
		))
	})
})
