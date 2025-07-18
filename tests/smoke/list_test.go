package smoke_test

import (
	"bytes"
	"encoding/json"
	"html/template"
	"strings"

	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/broker"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"github.com/PaesslerAG/jsonpath"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/types"
)

var _ = Describe("list", func() {
	var testVars map[string]string

	listResources := func(queryParams ...string) func(resourcePath string, resourcesMatch types.GomegaMatcher) {
		return func(resourcePath string, match types.GomegaMatcher) {
			if len(queryParams) > 0 {
				resourcePath += "?" + strings.Join(queryParams, "&")
			}

			tmpl, err := template.New("list").Parse(resourcePath)
			Expect(err).NotTo(HaveOccurred())
			var url bytes.Buffer
			Expect(tmpl.Execute(&url, testVars)).To(Succeed())

			cfCurlOutput, err := sessionOutput(helpers.Cf("curl", url.String()))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfCurlOutput).To(match)
		}
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

		testVars = map[string]string{}

		var err error
		testVars["orgGUID"], err = sessionOutput(helpers.Cf("org", sharedData.OrgName, "--guid"))
		Expect(err).NotTo(HaveOccurred())

		testVars["appGUID"], err = sessionOutput(helpers.Cf("app", sharedData.BuildpackAppName, "--guid"))
		Expect(err).NotTo(HaveOccurred())

		appPackageJSON, err := sessionOutput(helpers.Cf("curl", "/v3/packages?app_guids="+testVars["appGUID"]))
		Expect(err).NotTo(HaveOccurred())
		testVars["packageGUID"] = jsonGet("$.resources[0].guid", appPackageJSON)

		appRouteJSON, err := sessionOutput(helpers.Cf("curl", "/v3/apps/"+testVars["appGUID"]+"/routes"))
		Expect(err).NotTo(HaveOccurred())
		testVars["routeGUID"] = jsonGet("$.resources[0].guid", appRouteJSON)
	})

	DescribeTable("authorised users get the resources",
		listResources(),
		Entry("apps", "/v3/apps", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("app droplets", "/v3/apps/{{.appGUID}}/droplets", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("app processes", "/v3/apps/{{.appGUID}}/processes", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("app routes", "/v3/apps/{{.appGUID}}/routes", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("app packages", "/v3/apps/{{.appGUID}}/packages", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("builds", "/v3/builds", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("buildpacks", "/v3/buildpacks", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("deployments", "/v3/deployments", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("domains", "/v3/domains", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("droplets", "/v3/droplets", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("orgs", "/v3/organizations", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("org domains", "/v3/organizations/{{.orgGUID}}/domains", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("packages", "/v3/packages", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("package droplets", "/v3/packages/{{.packageGUID}}/droplets", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("processes", "/v3/processes", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("routes", "/v3/routes", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("routes", "/v3/routes/{{.routeGUID}}/destinations", MatchJSONPath("$.destinations", Not(BeEmpty()))),
		Entry("roles", "/v3/roles", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("service_instances", "/v3/service_instances", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("service_credential_bindings", "/v3/service_credential_bindings", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("service brokers", "/v3/service_brokers", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("service offerings", "/v3/service_offerings", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("service plans", "/v3/service_plans", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("spaces", "/v3/spaces", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("tasks", "/v3/tasks", MatchJSONPath("$.resources", Not(BeEmpty()))),
		Entry("app tasks", "/v3/apps/{{.appGUID}}/tasks", MatchJSONPath("$.resources", Not(BeEmpty()))),
	)

	When("paging params are provided", func() {
		DescribeTable("authorised users get the resources",
			listResources("per_page=1"),
			Entry("apps", "/v3/apps", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("app droplets", "/v3/apps/{{.appGUID}}/droplets", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("app processes", "/v3/apps/{{.appGUID}}/processes", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("app routes", "/v3/apps/{{.appGUID}}/routes", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("app packages", "/v3/apps/{{.appGUID}}/packages", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("builds", "/v3/builds", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("buildpacks", "/v3/buildpacks", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("deployments", "/v3/deployments", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("domains", "/v3/domains", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("droplets", "/v3/droplets", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("orgs", "/v3/organizations", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("org domains", "/v3/organizations/{{.orgGUID}}/domains", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("packages", "/v3/packages", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("package droplets", "/v3/packages/{{.packageGUID}}/droplets", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("processes", "/v3/processes", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("routes", "/v3/routes", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("roles", "/v3/roles", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("service_instances", "/v3/service_instances", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("service_credential_bindings", "/v3/service_credential_bindings", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("service brokers", "/v3/service_brokers", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("service offerings", "/v3/service_offerings", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("service plans", "/v3/service_plans", MatchJSONPath("$.resources", HaveLen(1))),
			Entry("spaces", "/v3/spaces", MatchJSONPath("$.resources", HaveLen(1))),
		)
	})

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
			listResources(),
			Entry("apps", "/v3/apps", MatchJSONPath("$.resources", BeEmpty())),
			Entry("builds", "/v3/builds", MatchJSONPath("$.resources", BeEmpty())),
			Entry("deployments", "/v3/deployments", MatchJSONPath("$.resources", BeEmpty())),
			Entry("droplets", "/v3/droplets", MatchJSONPath("$.resources", BeEmpty())),
			Entry("orgs", "/v3/organizations", MatchJSONPath("$.resources", BeEmpty())),
			Entry("packages", "/v3/packages", MatchJSONPath("$.resources", BeEmpty())),
			Entry("processes", "/v3/processes", MatchJSONPath("$.resources", BeEmpty())),
			Entry("routes", "/v3/routes", MatchJSONPath("$.resources", BeEmpty())),
			Entry("service_instances", "/v3/service_instances", MatchJSONPath("$.resources", BeEmpty())),
			Entry("service_credential_bindings", "/v3/service_credential_bindings", MatchJSONPath("$.resources", BeEmpty())),
			Entry("spaces", "/v3/spaces", MatchJSONPath("$.resources", BeEmpty())),
			Entry("tasks", "/v3/tasks", MatchJSONPath("$.resources", BeEmpty())),
		)

		DescribeTable("gets the global resources",
			listResources(),
			Entry("buildpacks", "/v3/buildpacks", MatchJSONPath("$.resources", Not(BeEmpty()))),
			Entry("domains", "/v3/domains", MatchJSONPath("$.resources", Not(BeEmpty()))),
			Entry("service brokers", "/v3/service_brokers", MatchJSONPath("$.resources", Not(BeEmpty()))),
			Entry("service offerings", "/v3/service_offerings", MatchJSONPath("$.resources", Not(BeEmpty()))),
			Entry("service plans", "/v3/service_plans", MatchJSONPath("$.resources", Not(BeEmpty()))),
		)
	})
})

func jsonGet(path string, jsonString string) string {
	GinkgoHelper()

	var obj any
	Expect(json.Unmarshal([]byte(jsonString), &obj)).To(Succeed())

	value, err := jsonpath.Get(path, obj)
	Expect(err).NotTo(HaveOccurred())

	strValue, ok := value.(string)
	Expect(ok).To(BeTrue())

	return strValue
}
