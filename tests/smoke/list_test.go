package smoke_test

import (
	"code.cloudfoundry.org/korifi/tests/helpers"
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
		Expect(cfCurlOutput).To(MatchJSONPath("$.resources", resourcesMatch))
	}

	BeforeEach(func() {
		Expect(helpers.Cf("run-task", sharedData.BuildpackAppName, "-c", "sleep 120")).To(Exit(0))

		upsiName := uuid.NewString()
		Expect(helpers.Cf("create-user-provided-service", upsiName)).To(Exit(0))
		Expect(helpers.Cf("bind-service", sharedData.BuildpackAppName, upsiName)).To(Exit(0))
	})

	DescribeTable("authorised users get the resources",
		listResources,
		Entry("apps", "apps", Not(BeEmpty())),
		Entry("packages", "packages", Not(BeEmpty())),
		Entry("processes", "processes", Not(BeEmpty())),
		Entry("routes", "routes", Not(BeEmpty())),
		Entry("service_instances", "service_instances", Not(BeEmpty())),
		Entry("service_credential_bindings", "service_credential_bindings", Not(BeEmpty())),
		Entry("tasks", "tasks", Not(BeEmpty())),
	)

	When("the user is not allowed to list", func() {
		BeforeEach(func() {
			serviceAccountFactory := helpers.NewServiceAccountFactory(sharedData.RootNamespace)
			userName := uuid.NewString()
			userToken := serviceAccountFactory.CreateServiceAccount(userName)
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

		DescribeTable("unauthorised users get empty resources list",
			listResources,
			Entry("apps", "apps", BeEmpty()),
			Entry("packages", "packages", BeEmpty()),
			Entry("processes", "processes", BeEmpty()),
			Entry("routes", "routes", BeEmpty()),
			Entry("service_instances", "service_instances", BeEmpty()),
			Entry("service_credential_bindings", "service_credential_bindings", BeEmpty()),
			Entry("tasks", "tasks", BeEmpty()),
		)
	})
})
