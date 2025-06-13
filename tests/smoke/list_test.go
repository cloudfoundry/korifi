package smoke_test

import (
	"fmt"

	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/broker"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/types"
)

var _ = Describe("list", func() {
	listResources := func(resourceType string, resourcesMatch types.GomegaMatcher) {
		cfCurlOutput, err := sessionOutput(helpers.Cf("curl", "/v3/"+resourceType))
		Expect(err).NotTo(HaveOccurred())
		Expect(cfCurlOutput).To(MatchJSONPath("$.resources", resourcesMatch), fmt.Sprintf("JSON output: %s", cfCurlOutput))
	}

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

		Expect(helpers.Cf("run-task", sharedData.BuildpackAppName, "-c", "sleep 120")).To(Exit(0))

		appName := uuid.NewString()
		Expect(helpers.Cf("create-app", appName)).To(Exit(0))

		upsiName := uuid.NewString()
		Expect(helpers.Cf("create-user-provided-service", upsiName)).To(Exit(0))
		Expect(helpers.Cf("bind-service", appName, upsiName)).To(Exit(0))
	})

	DescribeTable("authorised users get the resources",
		listResources,
		Entry("apps", "apps", Not(BeEmpty())),
		Entry("builds", "builds", Not(BeEmpty())),
		Entry("buildpacks", "buildpacks", Not(BeEmpty())),
		Entry("deployments", "deployments", Not(BeEmpty())),
		Entry("domains", "domains", Not(BeEmpty())),
		Entry("droplets", "droplets", Not(BeEmpty())),
		Entry("orgs", "organizations", Not(BeEmpty())),
		Entry("packages", "packages", Not(BeEmpty())),
		Entry("processes", "processes", Not(BeEmpty())),
		Entry("routes", "routes", Not(BeEmpty())),
		Entry("routes", "routes", Not(BeEmpty())),
		Entry("service_instances", "service_instances", Not(BeEmpty())),
		Entry("service_credential_bindings", "service_credential_bindings", Not(BeEmpty())),
		Entry("service brokers", "service_brokers", Not(BeEmpty())),
		Entry("service offerings", "service_offerings", Not(BeEmpty())),
		Entry("service plans", "service_plans", Not(BeEmpty())),
		Entry("spaces", "spaces", Not(BeEmpty())),
		Entry("tasks", "tasks", Not(BeEmpty())),
	)

	When("the user has no space roles", func() {
		BeforeEach(func() {
			serviceAccountFactory := helpers.NewServiceAccountFactory(sharedData.RootNamespace)
			userName := uuid.NewString()
			userToken := serviceAccountFactory.CreateRootNsUserServiceAccount(userName)
			helpers.NewFlock(sharedData.FLockPath).Execute(func() {
				helpers.AddUserToKubeConfig(userName, userToken)
			})

			DeferCleanup(func() {
				helpers.NewFlock(sharedData.FLockPath).Execute(func() {
					helpers.RemoveUserFromKubeConfig(userName)
				})
				serviceAccountFactory.DeleteServiceAccount(userName)
			})

			Expect(helpers.Cf("auth", userName)).To(Exit(0))
		})

		DescribeTable("gets empty resources list for non-global resources",
			listResources,
			Entry("apps", "apps", BeEmpty()),
			Entry("builds", "builds", BeEmpty()),
			Entry("deployments", "deployments", BeEmpty()),
			Entry("droplets", "droplets", BeEmpty()),
			Entry("orgs", "organizations", BeEmpty()),
			Entry("packages", "packages", BeEmpty()),
			Entry("processes", "processes", BeEmpty()),
			Entry("routes", "routes", BeEmpty()),
			Entry("service_instances", "service_instances", BeEmpty()),
			Entry("service_credential_bindings", "service_credential_bindings", BeEmpty()),
			Entry("spaces", "spaces", BeEmpty()),
			Entry("tasks", "tasks", BeEmpty()),
		)

		DescribeTable("gets the global resources",
			listResources,
			Entry("buildpacks", "buildpacks", Not(BeEmpty())),
			Entry("domains", "domains", Not(BeEmpty())),
			Entry("service brokers", "service_brokers", Not(BeEmpty())),
			Entry("service offerings", "service_offerings", Not(BeEmpty())),
			Entry("service plans", "service_plans", Not(BeEmpty())),
		)
	})
})
