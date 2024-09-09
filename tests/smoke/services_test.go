package smoke_test

import (
	"code.cloudfoundry.org/korifi/tests/helpers"

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
			helpers.GetInClusterURL(getAppGUID(brokerAppName)),
		)).To(Exit(0))
	})

	AfterEach(func() {
		cleanupBroker(brokerName)
	})

	Describe("cf create-service", func() {
		It("creates a managed service", func() {
			session := helpers.Cf("create-service", "sample-service", "sample", "-b", brokerName, uuid.NewString())
			Expect(session).To(Exit(0))
		})
	})
})
