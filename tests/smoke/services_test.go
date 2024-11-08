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

var _ = Describe("Services", func() {
	var brokerName string

	BeforeEach(func() {
		brokerName = uuid.NewString()
		Expect(helpers.Cf(
			"create-service-broker",
			brokerName,
			"broker-user",
			"broker-password",
			helpers.GetInClusterURL(getAppGUID(sharedData.BrokerAppName)),
		)).To(Exit(0))
		Expect(helpers.Cf("enable-service-access", "sample-service", "-b", brokerName)).To(Exit(0))
	})

	AfterEach(func() {
		broker.NewCatalogDeleter(sharedData.RootNamespace).ForBrokerName(brokerName).Delete()
	})

	Describe("cf create-service", func() {
		It("creates a managed service", func() {
			session := helpers.Cf("create-service", "sample-service", "sample", uuid.NewString(), "-b", brokerName)
			Expect(session).To(Exit(0))
		})
	})

	Describe("cf delete-service", func() {
		var serviceName string

		BeforeEach(func() {
			serviceName = uuid.NewString()
			session := helpers.Cf("create-service", "sample-service", "sample", serviceName, "-b", brokerName)
			Expect(session).To(Exit(0))
		})

		It("deletes the managed service", func() {
			session := helpers.Cf("delete-service", "-f", serviceName)
			Expect(session).To(Exit(0))
		})
	})

	Describe("cf services", func() {
		var serviceName string

		BeforeEach(func() {
			serviceName = uuid.NewString()
			session := helpers.Cf("create-service", "sample-service", "sample", serviceName, "-b", brokerName)
			Expect(session).To(Exit(0))
		})

		It("lists services", func() {
			session := helpers.Cf("services")
			Expect(session).To(Exit(0))

			lines := it.MustCollect(it.LinesString(session.Out))
			Expect(lines).To(ContainElement(
				matchSubstrings(serviceName, "sample-service", brokerName),
			))
		})
	})
})
