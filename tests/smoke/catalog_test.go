package smoke_test

import (
	"strings"

	"code.cloudfoundry.org/korifi/tests/helpers"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/types"
)

var _ = Describe("Service Catalog", func() {
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

	Describe("cf service-brokers", func() {
		It("lists service brokers", func() {
			session := helpers.Cf("service-brokers")
			Expect(session).To(Exit(0))

			lines, _ := it.Collect2(it.LinesString(session.Out))
			Expect(lines).To(ContainElement(
				matchSubstrings(brokerName, helpers.GetInClusterURL(getAppGUID(brokerAppName)))))
		})
	})

	Describe("cf delete-service-broker", func() {
		It("deletes the service broker", func() {
			session := helpers.Cf("delete-service-broker", "-f", brokerName)
			Expect(session).To(Exit(0))

			session = helpers.Cf("service-brokers")
			Expect(session).To(Exit(0))

			lines, _ := it.Collect2(it.LinesString(session.Out))
			Expect(lines).NotTo(ContainElement(ContainSubstring(brokerName)))
		})
	})

	Describe("cf service-access", func() {
		It("lists service access settings", func() {
			session := helpers.Cf("service-access", "-b", brokerName)
			Expect(session).To(Exit(0))

			lines, _ := it.Collect2(it.LinesString(session.Out))
			Expect(lines).To(ContainElements(
				matchSubstrings("sample-service", "sample", "none"),
			))
		})
	})

	Describe("cf enable-service-access", func() {
		It("enables the service access", func() {
			session := helpers.Cf("enable-service-access", "sample-service", "-b", brokerName)
			Expect(session).To(Exit(0))

			session = helpers.Cf("service-access")
			Expect(session).To(Exit(0))

			lines, _ := it.Collect2(it.LinesString(session.Out))
			Expect(lines).To(ContainElements(
				matchSubstrings("sample-service", "sample", "all"),
			))
		})
	})

	Describe("cf marketplace", func() {
		It("lists the service catalog", func() {
			session := helpers.Cf("marketplace", "-b", brokerName, "--show-unavailable")
			Expect(session).To(Exit(0))

			lines, _ := it.Collect2(it.LinesString(session.Out))
			Expect(lines).To(ContainElement(
				matchSubstrings("sample-service", "A sample service that does nothing", brokerName),
			))
		})
	})
})

func matchSubstrings(substrings ...string) types.GomegaMatcher {
	return MatchRegexp(strings.Join(substrings, ".*"))
}
